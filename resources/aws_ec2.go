// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sns"
	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("aws:ec2", func() Res { return &AwsEc2Res{} })
}

const (
	// AwsPrefix is a const which gets prepended onto object names. We can only use
	// alphanumeric chars, underscores and hyphens for sns topics and cloud watch rules.
	AwsPrefix = "_mgmt-"
	// Ec2Prefix is added to the names of sns and cloudwatch objects.
	Ec2Prefix = AwsPrefix + "ec2-"
	// SnsPrefix gets prepended onto the sns topic.
	SnsPrefix = Ec2Prefix + "sns-"
	// SnsTopicName is the name of the sns topic created by snsMakeTopic.
	SnsTopicName = SnsPrefix + "events"
	// httpProto is used to tell sns that the subscriber uses the http protocol.
	httpProto = "http"
	// waitTimeout is the duration in seconds of the timeout context in CheckApply.
	waitTimeout = 400
)

// AwsRegions is a list of all AWS regions generated using ec2.DescribeRegions.
// cn-north-1 and us-gov-west-1 are not returned, probably due to security.
// List available at http://docs.aws.amazon.com/general/latest/gr/rande.html
var AwsRegions = []string{
	"ap-northeast-1",
	"ap-northeast-2",
	"ap-south-1",
	"ap-southeast-1",
	"ap-southeast-2",
	"ca-central-1",
	"cn-north-1",
	"eu-central-1",
	"eu-west-1",
	"eu-west-2",
	"sa-east-1",
	"us-east-1",
	"us-east-2",
	"us-gov-west-1",
	"us-west-1",
	"us-west-2",
}

// AwsEc2Res is an AWS EC2 resource. In order to create a client session, your
// AWS credentials must be present in ~/.aws - For detailed instructions see
// http://docs.aws.amazon.com/cli/latest/userguide/cli-config-files.html
type AwsEc2Res struct {
	BaseRes `yaml:",inline"`
	State   string `yaml:"state"`   // state: running, stopped, terminated
	Region  string `yaml:"region"`  // region must match an element of AwsRegions
	Type    string `yaml:"type"`    // type of ec2 instance, eg: t2.micro
	ImageID string `yaml:"imageid"` // imageid must be available on the chosen region

	WatchEndpoint   string `yaml:"watchendpoint"`   // the public url of the sns endpoint, eg: http://server:12345/
	WatchListenAddr string `yaml:"watchlistenaddr"` // the local address or port that the sns listens on, eg: 10.0.0.0:23456 or 23456
	// UserData is used to run bash and cloud-init commands on first launch.
	// See http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/user-data.html
	// for documantation and examples.
	UserData string `yaml:"userdata"`

	client *ec2.EC2 // client session for AWS API calls

	snsEndpoint *http.Server // http server for SNS endpoint
	snsTopicArn string       // the unique id of the SNS topic

	awsChan   chan *chanStruct // channel used to send events and errors to Watch()
	closeChan chan struct{}    // channel used to cancel context when it's time to shut down
	wg        *sync.WaitGroup  // waitgroup for goroutines in Watch()
}

// chanStruct defines the type for a channel used to pass events and errors to watch.
type chanStruct struct {
	str string
	err error
}

// postData is the format of the messages received and decoded by postHandler().
type postData struct {
	Type    string `json:"Type"`
	Token   string `json:"Token"`
	Message string `json:"Message"`
}

// Default returns some sensible defaults for this resource.
func (obj *AwsEc2Res) Default() Res {
	return &AwsEc2Res{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *AwsEc2Res) Validate() error {
	if obj.State != "running" && obj.State != "stopped" && obj.State != "terminated" {
		return fmt.Errorf("state must be 'running', 'stopped' or 'terminated'")
	}

	// compare obj.Region to the list of available AWS endpoints.
	validRegion := false
	for _, region := range AwsRegions {
		if obj.Region == region {
			validRegion = true
			break
		}
	}
	if !validRegion {
		return fmt.Errorf("region must be a valid AWS endpoint")
	}

	// check the instance type
	// there is currently no api call to enumerate available instance types
	if obj.Type == "" {
		return fmt.Errorf("no instance type specified")
	}

	// check imageId against a list of available images
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(obj.Region),
	})
	if err != nil {
		return errwrap.Wrapf(err, "error creating session")
	}
	client := ec2.New(sess)

	imagesInput := &ec2.DescribeImagesInput{}
	images, err := client.DescribeImages(imagesInput)
	if err != nil {
		return errwrap.Wrapf(err, "error describing images")
	}
	validImage := false
	for _, image := range images.Images {
		if obj.ImageID == *image.ImageId {
			validImage = true
			break
		}
	}
	if !validImage {
		return fmt.Errorf("imageid must be a valid ami available in the specified region")
	}

	if obj.WatchEndpoint == "" && obj.WatchListenAddr != "" {
		return fmt.Errorf("you must set watchendpoint with watchlistenaddr to use http watch")
	}
	if obj.WatchEndpoint != "" && obj.WatchListenAddr == "" {
		return fmt.Errorf("you must set watchendpoint with watchlistenaddr to use http watch")
	}

	return obj.BaseRes.Validate()
}

// Init initializes the resource.
func (obj *AwsEc2Res) Init() error {
	// create a client session for the AWS API
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(obj.Region),
	})
	if err != nil {
		return errwrap.Wrapf(err, "error creating session")
	}
	obj.client = ec2.New(sess)

	obj.awsChan = make(chan *chanStruct)
	obj.closeChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}

	if obj.WatchListenAddr != "" {
		// set up the endpoint server
		obj.snsEndpoint = obj.snsServer()
		// make sns client
		snsClient, err := obj.snsClient()
		if err != nil {
			return errwrap.Wrapf(err, "error making sns client")
		}
		// make the sns topic
		snsTopicArn, err := obj.snsMakeTopic(snsClient)
		if err != nil {
			return errwrap.Wrapf(err, "error making sns topic")
		}
		// We need to save the topicArn for when it's time to delete the topic.
		// Otherwise, we would have to loop through every topic to find it.
		obj.snsTopicArn = snsTopicArn
	}

	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *AwsEc2Res) Watch() error {
	if obj.WatchListenAddr != "" {
		return obj.snsWatch()
	}
	return obj.longpollWatch()
}

// longpollWatch uses the ec2 api's built in methods to watch ec2 resource state.
func (obj *AwsEc2Res) longpollWatch() error {
	send := false
	var exit *error
	if err := obj.Running(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.TODO())
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		select {
		case <-obj.closeChan:
			cancel()
		}
	}()
	obj.wg.Add(1)
	defer obj.wg.Wait()
	go func() {
		defer obj.wg.Done()
		defer close(obj.awsChan)
		for {
			diInput := &ec2.DescribeInstancesInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("tag:Name"),
						Values: []*string{aws.String(obj.prependName())},
					},
					{
						Name: aws.String("instance-state-name"),
						Values: []*string{
							aws.String("pending"),
							aws.String("running"),
							aws.String("stopping"),
							aws.String("stopped"),
						},
					},
				},
			}
			diOutput, err := obj.client.DescribeInstances(diInput)
			if err != nil {
				select {
				case obj.awsChan <- &chanStruct{
					err: errwrap.Wrapf(err, "error describing instances"),
				}:
				case <-obj.closeChan:
				}
				return
			}
			if obj.State == "running" {
				stoppedInput := &ec2.DescribeInstancesInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("tag:Name"),
							Values: []*string{aws.String(obj.prependName())},
						},
						{
							Name: aws.String("instance-state-name"),
							Values: []*string{
								aws.String("stopped"),
							},
						},
					},
				}
				stoppedOutput, err := obj.client.DescribeInstances(stoppedInput)
				if err != nil {
					select {
					case obj.awsChan <- &chanStruct{
						err: errwrap.Wrapf(err, "error describing instances"),
					}:
					case <-obj.closeChan:
					}
					return
				}
				if len(diOutput.Reservations) == 1 && len(stoppedOutput.Reservations) == 0 {
					waitInput := &ec2.DescribeInstancesInput{
						InstanceIds: []*string{diOutput.Reservations[0].Instances[0].InstanceId},
						Filters: []*ec2.Filter{
							{
								Name: aws.String("instance-state-name"),
								Values: []*string{
									aws.String("stopped"),
									aws.String("terminated"),
								},
							},
						},
					}
					log.Printf("%s: Watching: %s", obj, *diOutput.Reservations[0].Instances[0].InstanceId)
					if err := obj.client.WaitUntilInstanceStoppedWithContext(ctx, waitInput); err != nil {
						if aerr, ok := err.(awserr.Error); ok {
							if aerr.Code() == request.CanceledErrorCode {
								log.Printf("%s: Request cancelled", obj)
							}
						}
						select {
						case obj.awsChan <- &chanStruct{
							err: errwrap.Wrapf(err, "unknown error waiting for instance to stop"),
						}:
						case <-obj.closeChan:
						}
						return
					}
					stateOutput, err := obj.client.DescribeInstances(diInput)
					if err != nil {
						select {
						case obj.awsChan <- &chanStruct{
							err: errwrap.Wrapf(err, "error describing instances"),
						}:
						case <-obj.closeChan:
						}
						return
					}
					var stateName string
					if len(stateOutput.Reservations) == 1 {
						stateName = *stateOutput.Reservations[0].Instances[0].State.Name
					}
					if len(stateOutput.Reservations) == 0 || (len(stateOutput.Reservations) == 1 && stateName != "running") {
						select {
						case obj.awsChan <- &chanStruct{
							str: "stopped",
						}:
						case <-obj.closeChan:
							return
						}
					}
				}
			}
			if obj.State == "stopped" {
				runningInput := &ec2.DescribeInstancesInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("tag:Name"),
							Values: []*string{aws.String(obj.prependName())},
						},
						{
							Name: aws.String("instance-state-name"),
							Values: []*string{
								aws.String("running"),
							},
						},
					},
				}
				runningOutput, err := obj.client.DescribeInstances(runningInput)
				if err != nil {
					select {
					case obj.awsChan <- &chanStruct{
						err: errwrap.Wrapf(err, "error describing instances"),
					}:
					case <-obj.closeChan:
					}
					return
				}
				if len(diOutput.Reservations) == 1 && len(runningOutput.Reservations) == 0 {
					waitInput := &ec2.DescribeInstancesInput{
						InstanceIds: []*string{diOutput.Reservations[0].Instances[0].InstanceId},
						Filters: []*ec2.Filter{
							{
								Name:   aws.String("instance-state-name"),
								Values: []*string{aws.String("running")},
							},
						},
					}
					log.Printf("%s: watching: %s", obj, *diOutput.Reservations[0].Instances[0].InstanceId)
					if err := obj.client.WaitUntilInstanceRunningWithContext(ctx, waitInput); err != nil {
						if aerr, ok := err.(awserr.Error); ok {
							if aerr.Code() == request.CanceledErrorCode {
								log.Printf("%s: Request cancelled", obj)
							}
						}
						select {
						case obj.awsChan <- &chanStruct{
							err: errwrap.Wrapf(err, "unknown error waiting for instance to start"),
						}:
						case <-obj.closeChan:
						}
						return
					}
					stateOutput, err := obj.client.DescribeInstances(diInput)
					if err != nil {
						select {
						case obj.awsChan <- &chanStruct{
							err: errwrap.Wrapf(err, "error describing instances"),
						}:
						case <-obj.closeChan:
						}
						return
					}
					var stateName string
					if len(stateOutput.Reservations) == 1 {
						stateName = *stateOutput.Reservations[0].Instances[0].State.Name
					}
					if len(stateOutput.Reservations) == 0 || (len(stateOutput.Reservations) == 1 && stateName != "stopped") {
						select {
						case obj.awsChan <- &chanStruct{
							str: "running",
						}:
						case <-obj.closeChan:
							return
						}
					}
				}
			}
			if obj.State == "terminated" {
				if err := obj.client.WaitUntilInstanceExistsWithContext(ctx, diInput); err != nil {
					if aerr, ok := err.(awserr.Error); ok {
						if aerr.Code() == request.CanceledErrorCode {
							log.Printf("%s: Request cancelled", obj)
						}
					}
					select {
					case obj.awsChan <- &chanStruct{
						err: errwrap.Wrapf(err, "unknown error waiting for instance to exist"),
					}:
					case <-obj.closeChan:
					}
					return
				}
				stateOutput, err := obj.client.DescribeInstances(diInput)
				if err != nil {
					select {
					case obj.awsChan <- &chanStruct{
						err: errwrap.Wrapf(err, "error describing instances"),
					}:
					case <-obj.closeChan:
					}
					return
				}
				if len(stateOutput.Reservations) == 1 {
					{
						select {
						case obj.awsChan <- &chanStruct{
							str: "exists",
						}:
						case <-obj.closeChan:
							return
						}
					}
				}
			}
			select {
			case <-obj.closeChan:
				return
			default:
			}
		}
	}()
	for {
		select {
		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				close(obj.closeChan)
				return *exit
			}
		case msg, ok := <-obj.awsChan:
			if !ok {
				return *exit
			}
			if err := msg.err; err != nil {
				return err
			}
			log.Printf("%s: State: %s", obj, msg.str)
			obj.StateOK(false)
			send = true
		}
		if send {
			send = false
			obj.Event()
		}
	}
}

// snsWatch uses amazon cloudwatch events and simple notification service to
// detect ec2 instance state changes.
func (obj *AwsEc2Res) snsWatch() error {
	send := false
	var exit *error
	var err error
	// make sns client
	snsClient, err := obj.snsClient()
	if err != nil {
		return errwrap.Wrapf(err, "error making sns client")
	}
	// start the endpoint
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		log.Printf("%s: Starting SNS Endpoint", obj)
		if err := obj.snsEndpoint.ListenAndServe(); err != nil {
			select {
			case obj.awsChan <- &chanStruct{
				err: errwrap.Wrapf(err, "sns server error"),
			}:
			case <-obj.closeChan:
			}
		}
	}()
	// subscribe endpoint to the topic
	if err = obj.snsSubscribe(snsClient); err != nil {
		return errwrap.Wrapf(err, "error subscribing to sns topic")
	}
	for {
		select {
		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				close(obj.closeChan)
				return *exit
			}
		case msg, ok := <-obj.awsChan:
			if !ok {
				return *exit
			}
			if err := msg.err; err != nil {
				return err
			}
			log.Printf("%s: State: %s", obj, msg.str)
			obj.StateOK(false)
			send = true
		}
		if send {
			send = false
			obj.Event()
		}
	}
}

// CheckApply method for AwsEc2 resource.
func (obj *AwsEc2Res) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s: CheckApply(%t)", obj, apply)

	diInput := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(obj.prependName())},
			},
			{
				Name: aws.String("instance-state-name"),
				Values: []*string{
					aws.String("running"),
					aws.String("pending"),
					aws.String("stopped"),
					aws.String("stopping"),
				},
			},
		},
	}
	diOutput, err := obj.client.DescribeInstances(&diInput)
	if err != nil {
		return false, errwrap.Wrapf(err, "error describing instances")
	}

	if len(diOutput.Reservations) < 1 && obj.State == "terminated" {
		return true, nil
	}
	if len(diOutput.Reservations) == 1 && *diOutput.Reservations[0].Instances[0].State.Name == obj.State {
		return true, nil
	}
	if !apply {
		return false, nil
	}

	if len(diOutput.Reservations) > 1 {
		return false, fmt.Errorf("too many reservations")
	}
	ctx, cancel := context.WithTimeout(context.TODO(), waitTimeout*time.Second)
	defer cancel()
	if len(diOutput.Reservations) == 1 {
		instanceID := diOutput.Reservations[0].Instances[0].InstanceId
		describeInput := &ec2.DescribeInstancesInput{
			InstanceIds: []*string{instanceID},
		}
		if len(diOutput.Reservations[0].Instances) > 1 {
			return false, fmt.Errorf("more than one instance was returned")
		}
		if obj.State == "running" {
			startInput := &ec2.StartInstancesInput{
				InstanceIds: []*string{instanceID},
			}
			_, err := obj.client.StartInstances(startInput)
			if err != nil {
				return false, errwrap.Wrapf(err, "error starting instance")
			}
			if err := obj.client.WaitUntilInstanceRunningWithContext(ctx, describeInput); err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == request.CanceledErrorCode {
						return false, errwrap.Wrapf(err, "timeout while waiting for instance to start")
					}
				}
				return false, errwrap.Wrapf(err, "unknown error waiting for instance to start")
			}
			log.Printf("%s: instance running", obj)
		}
		if obj.State == "stopped" {
			stopInput := &ec2.StopInstancesInput{
				InstanceIds: []*string{instanceID},
			}
			_, err := obj.client.StopInstances(stopInput)
			if err != nil {
				return false, errwrap.Wrapf(err, "error stopping instance")
			}
			if err := obj.client.WaitUntilInstanceStoppedWithContext(ctx, describeInput); err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == request.CanceledErrorCode {
						return false, errwrap.Wrapf(err, "timeout while waiting for instance to stop")
					}
				}
				return false, errwrap.Wrapf(err, "unknown error waiting for instance to stop")
			}
			log.Printf("%s: instance stopped", obj)
		}
		if obj.State == "terminated" {
			terminateInput := &ec2.TerminateInstancesInput{
				InstanceIds: []*string{instanceID},
			}
			_, err := obj.client.TerminateInstances(terminateInput)
			if err != nil {
				return false, errwrap.Wrapf(err, "error terminating instance")
			}
			if err := obj.client.WaitUntilInstanceTerminatedWithContext(ctx, describeInput); err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == request.CanceledErrorCode {
						return false, errwrap.Wrapf(err, "timeout while waiting for instance to terminate")
					}
				}
				return false, errwrap.Wrapf(err, "unknown error waiting for instance to terminate")
			}
			log.Printf("%s: instance terminated", obj)
		}
	}
	if len(diOutput.Reservations) < 1 && obj.State == "running" {
		runParams := &ec2.RunInstancesInput{
			ImageId:      aws.String(obj.ImageID),
			InstanceType: aws.String(obj.Type),
		}
		runParams.SetMinCount(1)
		runParams.SetMaxCount(1)
		if obj.UserData != "" {
			userData := base64.StdEncoding.EncodeToString([]byte(obj.UserData))
			runParams.SetUserData(userData)
		}
		runResult, err := obj.client.RunInstances(runParams)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not create instance")
		}
		_, err = obj.client.CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{runResult.Instances[0].InstanceId},
			Tags: []*ec2.Tag{
				&ec2.Tag{
					Key:   aws.String("Name"),
					Value: aws.String(obj.prependName()),
				},
			},
		})
		if err != nil {
			return false, errwrap.Wrapf(err, "could not create tags for instance")
		}

		describeInput := &ec2.DescribeInstancesInput{
			InstanceIds: []*string{runResult.Instances[0].InstanceId},
		}
		err = obj.client.WaitUntilInstanceRunningWithContext(ctx, describeInput)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == request.CanceledErrorCode {
					return false, errwrap.Wrapf(err, "timeout while waiting for instance to start")
				}
			}
			return false, errwrap.Wrapf(err, "unknown error waiting for instance to start")
		}
		log.Printf("%s: instance running", obj)
	}
	return false, nil
}

// AwsEc2UID is the UID struct for AwsEc2Res.
type AwsEc2UID struct {
	BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *AwsEc2Res) UIDs() []ResUID {
	x := &AwsEc2UID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *AwsEc2Res) GroupCmp(r Res) bool {
	_, ok := r.(*AwsEc2Res)
	if !ok {
		return false
	}
	return false
}

// Compare two resources and return if they are equivalent.
func (obj *AwsEc2Res) Compare(r Res) bool {
	// we can only compare AwsEc2Res to others of the same resource kind
	res, ok := r.(*AwsEc2Res)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) { // call base Compare
		return false
	}
	if obj.Name != res.Name {
		return false
	}
	if obj.State != res.State {
		return false
	}
	if obj.Region != res.Region {
		return false
	}
	if obj.Type != res.Type {
		return false
	}
	if obj.ImageID != res.ImageID {
		return false
	}
	if obj.UserData != res.UserData {
		return false
	}
	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *AwsEc2Res) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes AwsEc2Res // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*AwsEc2Res) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to AwsEc2Res")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = AwsEc2Res(raw) // restore from indirection with type conversion!
	return nil
}

func (obj *AwsEc2Res) prependName() string {
	return AwsPrefix + obj.GetName()
}

// snsClient returns a client for AWS SNS API calls.
func (obj *AwsEc2Res) snsClient() (*sns.SNS, error) {
	// make sns client
	snsSess, err := session.NewSession(&aws.Config{
		Region: aws.String(obj.Region),
	})
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating sns session")
	}
	return sns.New(snsSess), nil
}

// snsServer returns an http server used to listen for sns messages.
func (obj *AwsEc2Res) snsServer() *http.Server {
	addr := obj.WatchListenAddr
	// if addr is a port
	if _, err := strconv.Atoi(obj.WatchListenAddr); err == nil {
		addr = fmt.Sprintf(":%s", obj.WatchListenAddr)
	}
	handler := http.HandlerFunc(obj.snsPostHandler)
	return &http.Server{
		Addr:    addr,
		Handler: handler,
	}
}

// snsPostHandler listens for posts on the SNS Endpoint.
func (obj *AwsEc2Res) snsPostHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	// decode json
	decoder := json.NewDecoder(req.Body)
	var p postData
	err := decoder.Decode(&p)
	if err != nil {
		select {
		case obj.awsChan <- &chanStruct{
			err: errwrap.Wrapf(err, "error decoding post"),
		}:
		case <-obj.closeChan:
		}
		return
	}
	if p.Type == "SubscriptionConfirmation" {
		// we need a sns client to confirm the subscription
		snsClient, err := obj.snsClient()
		if err != nil {
			select {
			case obj.awsChan <- &chanStruct{
				err: errwrap.Wrapf(err, "error making sns client"),
			}:
			case <-obj.closeChan:
			}
		}
		// confirm the subscription
		csInput := &sns.ConfirmSubscriptionInput{
			Token:    aws.String(p.Token),
			TopicArn: aws.String(obj.snsTopicArn),
		}
		_, err = snsClient.ConfirmSubscription(csInput)
		if err != nil {
			select {
			case obj.awsChan <- &chanStruct{
				err: errwrap.Wrapf(err, "error confirming subscription"),
			}:
			case <-obj.closeChan:
			}
			return
		}
		log.Printf("%s: Subscription Confirmed", obj)
	}
}

// snsMakeTopic creates a topic on aws sns.
func (obj *AwsEc2Res) snsMakeTopic(snsClient *sns.SNS) (string, error) {
	// make topic
	topicInput := &sns.CreateTopicInput{
		Name: aws.String(SnsTopicName),
	}
	topic, err := snsClient.CreateTopic(topicInput)
	if err != nil {
		return "", err
	}
	log.Printf("%s: Created SNS Topic", obj)

	var snsTopicArn string
	if topic.TopicArn != nil {
		snsTopicArn = *topic.TopicArn
	}
	if snsTopicArn == "" {
		return "", fmt.Errorf("sns topic arn is empty")
	}
	return snsTopicArn, nil
}

// snsDeleteTopic deletes the sns topic.
func (obj *AwsEc2Res) snsDeleteTopic(snsClient *sns.SNS) error {
	// delete the topic
	dtInput := &sns.DeleteTopicInput{
		TopicArn: aws.String(obj.snsTopicArn),
	}
	log.Printf("%s: Deleting Topic", obj)
	if _, err := snsClient.DeleteTopic(dtInput); err != nil {
		return errwrap.Wrapf(err, "error deleting topic")
	}
	return nil
}

// snsSubscribe subscribes the endpoint to the sns topic.
func (obj *AwsEc2Res) snsSubscribe(snsClient *sns.SNS) error {
	// subscribe to the topic
	subInput := &sns.SubscribeInput{
		Endpoint: aws.String(obj.WatchEndpoint),
		Protocol: aws.String(httpProto),
		TopicArn: aws.String(obj.snsTopicArn),
	}
	_, err := snsClient.Subscribe(subInput)
	if err != nil {
		return err
	}
	log.Printf("%s: Created Subscription", obj)
	return nil
}

// snsUnsubscribe unsubscribes the endpoint from the sns topic.
func (obj *AwsEc2Res) snsUnsubscribe(snsClient *sns.SNS) error {
	// get the subscriptionArn
	lsInput := &sns.ListSubscriptionsByTopicInput{
		TopicArn: aws.String(obj.snsTopicArn),
	}
	lsOutput, err := snsClient.ListSubscriptionsByTopic(lsInput)
	if err != nil {
		return errwrap.Wrapf(err, "error listing subscriptions")
	}
	// unsubscribe the endpoint before we delete the topic
	if len(lsOutput.Subscriptions) > 0 {
		usInput := &sns.UnsubscribeInput{
			SubscriptionArn: lsOutput.Subscriptions[0].SubscriptionArn,
		}
		log.Printf("%s: Unsubscribing Endpoint", obj)
		if _, err := snsClient.Unsubscribe(usInput); err != nil {
			return errwrap.Wrapf(err, "error unsubscribing endpoint")
		}
	}
	return nil
}

// Close cleans up when we're done. This is needed to delete some of the AWS
// objects created for the SNS endpoint.
func (obj *AwsEc2Res) Close() error {
	var errList error
	if obj.WatchListenAddr != "" {
		// make sns client
		snsClient, err := obj.snsClient()
		if err != nil {
			errList = multierr.Append(errList, err)
		}
		// unsubscribe the endpoint
		if err := obj.snsUnsubscribe(snsClient); err != nil {
			errList = multierr.Append(errList, err)
		}
		// delete the topic
		if err := obj.snsDeleteTopic(snsClient); err != nil {
			errList = multierr.Append(errList, err)
		}
		// stop the http endpoint
		if err := obj.snsEndpoint.Shutdown(nil); err != nil {
			log.Printf("%s: Shutting Down Endpoint", obj)
			errList = multierr.Append(errList, err)
		}
	}
	if err := obj.BaseRes.Close(); err != nil {
		errList = multierr.Append(errList, err) // list of errors
	}
	return errList
}
