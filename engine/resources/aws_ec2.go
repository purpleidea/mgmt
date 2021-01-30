// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	cwe "github.com/aws/aws-sdk-go/service/cloudwatchevents"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sns"
)

func init() {
	engine.RegisterResource("aws:ec2", func() engine.Res { return &AwsEc2Res{} })
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
	// SnsSubscriptionProto is used to tell sns that the subscriber uses the http protocol.
	// TODO: add https support
	SnsSubscriptionProto = "http"
	// SnsServerShutdownTimeout is the maximum number of seconds to wait for the http server to shutdown gracefully.
	SnsServerShutdownTimeout = 30
	// SnsPolicy is the topic attribute that defines the security policy for the topic.
	SnsPolicy = "Policy"
	// SnsPolicySid is the friendly name of the policy statement.
	SnsPolicySid = CwePrefix + "publish"
	// SnsPolicyEffect allows the action(s) defined in the policy statement.
	SnsPolicyEffect = "Allow"
	// SnsPolicyService is the cloudwatch events security principal that we are granting the permission to.
	SnsPolicyService = "events.amazonaws.com"
	// SnsPolicyAction is the specific permission we are granting in the policy.
	SnsPolicyAction = "SNS:Publish"
	// SnsCertURLRegex is used to make sure we only download certificates
	// from amazon. This regex will match "https://sns.***.amazonaws.com/"
	// where *** represents any combination of words and hyphens, and will
	// match any aws region name, eg: ca-central-1.
	SnsCertURLRegex = `(^https:\/\/sns\.([\w\-])+\.amazonaws.com\/)`
	// CwePrefix gets prepended onto the cloudwatch rule name.
	CwePrefix = Ec2Prefix + "cw-"
	// CweRuleName is the name of the rule created by makeCloudWatchRule.
	CweRuleName = CwePrefix + "state"
	// CweRuleSource describes the resource type to monitor for cloudwatch events.
	CweRuleSource = "aws.ec2"
	// CweRuleDetailType describes the specific type of events to trigger cloudwatch.
	CweRuleDetailType = "EC2 Instance State-change Notification"
	// CweTargetID is used to tell cloudwatch events to target the sns service.
	CweTargetID = "sns"
	// CweTargetJSON is the json field that cloudwatch will send to our endpoint so we don't get more than we need.
	CweTargetJSON = "$.detail"
	// AwsErrExceededWaitAttempts is the awserr.Message() that gets sent with
	// the ResourceStateNotReady awserr.Code() when the waiters time out.
	AwsErrExceededWaitAttempts = "exceeded wait attempts"
	// AwsErrIncorrectInstanceState is the error returned when an action
	// cannot be completed due to the current instance state.
	AwsErrIncorrectInstanceState = "IncorrectInstanceState"
	// waitTimeout is the duration in seconds of the timeout context in CheckApply.
	waitTimeout = 400
	// nameKey is the name of the tag key that stores the instance name in ec2.Instance.
	// in ec2.Instance
	nameKey = "Name"
	// nameTag is used to define the name tag.
	nameTag = "tag:" + nameKey
)

//go:generate stringer -type=awsEc2Event -output=awsec2event_stringer.go

// awsEc2Event represents the contents of event messages sent via awsChan.
type awsEc2Event uint8

const (
	awsEc2EventNone awsEc2Event = iota
	awsEc2EventWatchReady
	awsEc2EventInstanceStopped
	awsEc2EventInstanceRunning
	awsEc2EventInstanceTerminated
	awsEc2EventInstanceExists
)

// AwsRegions is a list of all AWS regions generated using ec2.DescribeRegions.
// cn-north-1 and us-gov-west-1 are not returned, probably due to security. List
// available at http://docs.aws.amazon.com/general/latest/gr/rande.html
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
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	State   string `yaml:"state"`   // state: running, stopped, terminated
	Region  string `yaml:"region"`  // region must match an element of AwsRegions
	Type    string `yaml:"type"`    // type of ec2 instance, eg: t2.micro
	ImageID string `yaml:"imageid"` // imageid must be available on the chosen region

	WatchEndpoint   string `yaml:"watchendpoint"`   // the public url of the sns endpoint, eg: http://server:12345/
	WatchListenAddr string `yaml:"watchlistenaddr"` // the local address or port that the sns listens on, eg: 10.0.0.0:23456 or 23456
	// ErrorOnMalformedPost controls whether or not malformed HTTP post
	// requests, that cause JSON decoder errors, will also make the engine
	// shut down. If ErrorOnMalformedPost set to true and an error occurs,
	// Watch() will return the error and the engine will shut down.
	ErrorOnMalformedPost bool `yaml:"erroronmalformedpost"`

	// UserData is used to run bash and cloud-init commands on first launch.
	// See http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/user-data.html
	// for documantation and examples.
	UserData string `yaml:"userdata"`

	client *ec2.EC2 // client session for AWS API calls

	snsClient *sns.SNS // client for AWS SNS API calls
	// snsTopicArn requires looping through every topic to get,
	// so we save it here when we create the topic instead.
	snsTopicArn string

	cweClient *cwe.CloudWatchEvents // client for AWS CloudWatchEvents API calls

	awsChan   chan *chanStruct // channel used to send events and errors to Watch()
	closeChan chan struct{}    // channel used to cancel context when it's time to shut down
	wg        *sync.WaitGroup  // waitgroup for goroutines in Watch()

	// store read-only values for send/recv.
	PublicIPv4  string
	PrivateIPv4 string
	InstanceID  string
}

// chanStruct defines the type for a channel used to pass events and errors to
// watch.
type chanStruct struct {
	event awsEc2Event
	state string
	err   error
}

// snsPolicy denotes the structure of sns security policies.
type snsPolicy struct {
	Version   string         `json:"Version"`
	ID        string         `json:"Id"`
	Statement []snsStatement `json:"Statement"`
}

// snsStatement denotes the structure of sns security policy statements.
type snsStatement struct {
	Sid       string       `json:"Sid"`
	Effect    string       `json:"Effect"`
	Principal snsPrincipal `json:"Principal"`
	Action    interface{}  `json:"Action"`
	Resource  string       `json:"Resource"`
	Condition *struct {
		StringEquals *struct {
			AWSSourceOwner *string `json:"AWS:SourceOwner,omitempty"`
		} `json:"StringEquals,omitempty"`
	} `json:"Condition,omitempty"`
}

// snsPrincipal describes the aws or service account principal.
type snsPrincipal struct {
	AWS     string `json:"AWS,omitempty"`
	Service string `json:"Service,omitempty"`
}

// cloudWatchRule denotes the structure of cloudwatch rules.
type cloudWatchRule struct {
	Source     []string   `json:"source"`
	DetailType []string   `json:"detail-type"`
	Detail     ruleDetail `json:"detail"`
}

// ruleDetail is the structure of the detail field in cloudWatchRule.
type ruleDetail struct {
	State []string `json:"state"`
}

// postData is the format of the messages received and decoded by
// snsPostHandler().
type postData struct {
	Type             string `json:"Type"`
	MessageID        string `json:"MessageId"`
	Token            string `json:"Token"`
	TopicArn         string `json:"TopicArn"`
	Message          string `json:"Message"`
	SubscribeURL     string `json:"SubscribeURL"`
	Timestamp        string `json:"Timestamp"`
	SignatureVersion string `json:"SignatureVersion"`
	Signature        string `json:"Signature"`
	SigningCertURL   string `json:"SigningCertURL"`
}

// postMsg is used to unmarshal the postData message if it's an event
// notification.
type postMsg struct {
	InstanceID string `json:"instance-id"`
	State      string `json:"state"`
}

// Default returns some sensible defaults for this resource.
func (obj *AwsEc2Res) Default() engine.Res {
	return &AwsEc2Res{}
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

	return nil
}

// Init initializes the resource.
func (obj *AwsEc2Res) Init(init *engine.Init) error {
	obj.init = init // save for later

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

	// if we are using sns watch
	if obj.WatchListenAddr != "" {
		// make sns client
		snsSess, err := session.NewSession(&aws.Config{
			Region: aws.String(obj.Region),
		})
		if err != nil {
			return errwrap.Wrapf(err, "error creating sns session")
		}
		obj.snsClient = sns.New(snsSess)
		// make the sns topic
		snsTopicArn, err := obj.snsMakeTopic()
		if err != nil {
			return errwrap.Wrapf(err, "error making sns topic")
		}
		// save the topicArn for later use
		obj.snsTopicArn = snsTopicArn

		// make cloudwatch client
		cweSess, err := session.NewSession(&aws.Config{
			Region: aws.String(obj.Region),
		})
		if err != nil {
			return errwrap.Wrapf(err, "error creating cwe session")
		}
		obj.cweClient = cwe.New(cweSess)
		// make the cloudwatch rule event pattern
		// CweRuleDetail describes the instance states that will trigger events.
		CweRuleDetail := []string{"running", "stopped", "terminated"}
		eventPattern, err := obj.cweMakeEventPattern(CweRuleSource, CweRuleDetailType, CweRuleDetail)
		if err != nil {
			return err
		}
		// make the cloudwatch rule
		if err := obj.cweMakeRule(CweRuleName, eventPattern); err != nil {
			return errwrap.Wrapf(err, "error making cloudwatch rule")
		}
		// target cloudwatch rule to sns topic
		if err := obj.cweTargetRule(obj.snsTopicArn, CweTargetID, CweTargetJSON, CweRuleName); err != nil {
			return errwrap.Wrapf(err, "error targeting cloudwatch rule")
		}
		// authorize cloudwatch to publish on sns
		// This gets cleaned up in Close(), when the topic is deleted.
		if err := obj.snsAuthorizeCloudWatch(obj.snsTopicArn); err != nil {
			return errwrap.Wrapf(err, "error authorizing cloudwatch for sns")
		}
	}

	return nil
}

// Close cleans up when we're done. This is needed to delete some of the AWS
// objects created for the SNS endpoint.
func (obj *AwsEc2Res) Close() error {
	var errList error
	// clean up sns objects created by Init/snsWatch
	if obj.snsClient != nil {
		// delete the topic and associated subscriptions
		e1 := obj.snsDeleteTopic(obj.snsTopicArn)
		errList = errwrap.Append(errList, e1)
		// remove the target
		e2 := obj.cweRemoveTarget(CweTargetID, CweRuleName)
		errList = errwrap.Append(errList, e2)
		// delete the cloudwatch rule
		e3 := obj.cweDeleteRule(CweRuleName)
		errList = errwrap.Append(errList, e3)
	}

	return errList
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *AwsEc2Res) Watch() error {
	if obj.WatchListenAddr != "" {
		return obj.snsWatch()
	}
	return obj.longpollWatch()
}

// longpollWatch uses the ec2 api's built in methods to watch ec2 resource
// state.
func (obj *AwsEc2Res) longpollWatch() error {
	send := false

	// We tell the engine that we're running right away. This is not correct,
	// but the api doesn't have a way to signal when the waiters are ready.
	obj.init.Running() // when started, notify engine that we're running

	// cancellable context used for exiting cleanly
	ctx, cancel := context.WithCancel(context.TODO())

	// clean up when we're done
	defer obj.wg.Wait()
	defer close(obj.closeChan)

	// cancel our context if obj.closeChan closes
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		select {
		case <-obj.closeChan:
			cancel()
		}
	}()

	// monitor the resource and send the state to the channel
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(obj.awsChan)
		for {
			// get the instance with the name specified in the definition
			instance, err := describeInstanceByName(obj.client, obj.prependName())
			if err != nil {
				select {
				case obj.awsChan <- &chanStruct{
					err: errwrap.Wrapf(err, "error describing instance"),
				}:
				case <-obj.closeChan:
				}
				return
			}

			// wait for the instance state to change
			state, err := stateWaiter(ctx, instance, obj.client)
			if err != nil {
				select {
				case obj.awsChan <- &chanStruct{
					err: errwrap.Wrapf(err, "waiter error"),
				}:
				case <-obj.closeChan:
				}
				return
			}

			// send the state to the event processing loop
			select {
			case obj.awsChan <- &chanStruct{
				state: state,
			}:
			case <-obj.closeChan:
				return
			}
		}
	}()

	// process events from the goroutine
	for {
		select {
		case msg, ok := <-obj.awsChan:
			if !ok {
				return nil
			}
			if err := msg.err; err != nil {
				return err
			}
			switch msg.state {
			// send events to the engine, except empty and transitional states
			case "", ec2.InstanceStateNamePending, ec2.InstanceStateNameStopping:
				continue
			default:
				obj.init.Logf("State: %v", msg.state)
				send = true
			}

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// snsWatch uses amazon's SNS and CloudWatchEvents APIs to get instance state-
// change notifications pushed to the http endpoint (snsServer) set up below. In
// Init() a CloudWatch rule is created along with a corresponding SNS topic that
// it can publish to. snsWatch creates an http server which listens for messages
// published to the topic and processes them accordingly.
func (obj *AwsEc2Res) snsWatch() error {
	send := false
	defer obj.wg.Wait()
	// create the sns listener
	// closing is handled by http.Server.Shutdown in the defer func below
	listener, err := obj.snsListener(obj.WatchListenAddr)
	if err != nil {
		return errwrap.Wrapf(err, "error creating listener")
	}
	// set up the sns server
	snsServer := &http.Server{
		Handler: http.HandlerFunc(obj.snsPostHandler),
	}
	// close the listener and shutdown the sns server when we're done
	defer func() {
		ctx, cancel := context.WithTimeout(context.TODO(), SnsServerShutdownTimeout*time.Second)
		defer cancel()
		if err := snsServer.Shutdown(ctx); err != nil {
			if err != context.Canceled {
				obj.init.Logf("error stopping sns endpoint: %s", err)
				return
			}
			obj.init.Logf("sns server shutdown cancelled")
		}
	}()
	defer close(obj.closeChan)
	obj.wg.Add(1)
	// start the sns server
	go func() {
		defer obj.wg.Done()
		defer close(obj.awsChan)
		if err := snsServer.Serve(listener); err != nil {
			// when we shut down
			if err == http.ErrServerClosed {
				obj.init.Logf("Stopped SNS Endpoint")
				return
			}
			// any other error
			select {
			case obj.awsChan <- &chanStruct{
				err: errwrap.Wrapf(err, "sns server error"),
			}:
			case <-obj.closeChan:
			}
		}
	}()
	obj.init.Logf("Started SNS Endpoint")
	// Subscribing the endpoint to the topic needs to happen after starting
	// the http server, so that the server can process the subscription
	// confirmation. We won't drop incoming connections from aws by this
	// point, because we've already opened the server listener. In the
	// worst case scenario the incoming aws connections will be accepted
	// but will block until our http server finishes getting ready in
	// its goroutine.
	if err := obj.snsSubscribe(obj.WatchEndpoint, obj.snsTopicArn); err != nil {
		return errwrap.Wrapf(err, "error subscribing to sns topic")
	}
	// process events
	for {
		select {
		case msg, ok := <-obj.awsChan:
			if !ok {
				return nil
			}
			if err := msg.err; err != nil {
				return err
			}
			// snsPostHandler sends the ready message after the
			// subscription is confirmed. Once the subscription
			// is confirmed, we are ready to receive events, so we
			// can notify the engine that we're running.
			if msg.event == awsEc2EventWatchReady {
				obj.init.Running() // when started, notify engine that we're running
				continue
			}
			obj.init.Logf("State: %v", msg.event)
			send = true

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// CheckApply method for AwsEc2 resource.
func (obj *AwsEc2Res) CheckApply(apply bool) (bool, error) {
	obj.init.Logf("CheckApply(%t)", apply)

	// find the instance we need to check
	instance, err := describeInstanceByName(obj.client, obj.prependName())
	if err != nil {
		return false, errwrap.Wrapf(err, "error describing instance")
	}
	if instance == nil {
		return false, fmt.Errorf("instance is nil")
	}

	// store useful send/recv values when we return
	defer func() {
		// safely dereference the pointers (nil becomes "")
		obj.PublicIPv4 = aws.StringValue(instance.PublicIpAddress)
		obj.PrivateIPv4 = aws.StringValue(instance.PrivateIpAddress)
		obj.InstanceID = aws.StringValue(instance.InstanceId)
	}()

	// If the instance is in a transitional state, Watch will send a new event
	// when the instance finishes its transition. Since we can't apply the
	// desired state until the instance is stopped, running, or terminated, we
	// return and wait for the next event.
	switch aws.StringValue(instance.State.Name) {
	case ec2.InstanceStateNamePending, ec2.InstanceStateNameStopping:
		return false, nil
	default:
	}
	// if the instance is in the correct state, we're done
	if obj.State == aws.StringValue(instance.State.Name) {
		return true, nil
	}
	// if the instance is terminated and should be stopped, we're done
	if obj.State == ec2.InstanceStateNameStopped && aws.StringValue(instance.InstanceId) == "" {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	// If the instance is terminated or does not exist, and should be running,
	// launch it with the defined parameters.
	if obj.State == ec2.InstanceStateNameRunning && aws.StringValue(instance.InstanceId) == "" {
		runOutput, err := obj.client.RunInstances(&ec2.RunInstancesInput{
			ImageId:      aws.String(obj.ImageID),
			InstanceType: aws.String(obj.Type),
			MinCount:     aws.Int64(1),
			MaxCount:     aws.Int64(1),
			UserData:     aws.String(obj.UserData),
		})
		if err != nil {
			return false, errwrap.Wrapf(err, "error running instance")
		}
		// error if runInstances doesn't return exactly one instance
		if len(runOutput.Instances) != 1 {
			return false, fmt.Errorf("incorrect number of instances")
		}

		// add the name tag
		_, err = obj.client.CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{runOutput.Instances[0].InstanceId},
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(nameKey),
					Value: aws.String(obj.prependName()),
				},
			},
		})
		if err != nil {
			return false, errwrap.Wrapf(err, "could not create name tag")
		}

		// get the latest metadata
		instance, err = describeInstanceByID(obj.client, runOutput.Instances[0].InstanceId)
		if err != nil {
			return false, errwrap.Wrapf(err, "error describing instance")
		}
	}

	// Apply the correct state. If we just launched the instance, it's okay to
	// start it, as the function will return as normal, and execution will
	// continue onto the waiters.
	switch obj.State {
	case ec2.InstanceStateNameRunning:
		_, err = obj.client.StartInstances(&ec2.StartInstancesInput{
			InstanceIds: []*string{instance.InstanceId},
		})
	case ec2.InstanceStateNameStopped:
		_, err = obj.client.StopInstances(&ec2.StopInstancesInput{
			InstanceIds: []*string{instance.InstanceId},
		})
	case ec2.InstanceStateNameTerminated:
		_, err = obj.client.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: []*string{instance.InstanceId},
		})
	}
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			// if the instance is in a transitional state, return and wait for
			// the next event.
			if aerr.Code() == AwsErrIncorrectInstanceState {
				return false, nil
			}
		}
		return false, errwrap.Wrapf(err, "error applying state")
	}

	// context to cancel the waiter if it takes too long
	ctx, cancel := context.WithTimeout(context.TODO(), waitTimeout*time.Second)
	defer cancel()

	// wait until the state converges
	waitInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{instance.InstanceId},
	}
	switch obj.State {
	case ec2.InstanceStateNameRunning:
		err = obj.client.WaitUntilInstanceRunningWithContext(ctx, waitInput)
	case ec2.InstanceStateNameStopped:
		err = obj.client.WaitUntilInstanceStoppedWithContext(ctx, waitInput)
	case ec2.InstanceStateNameTerminated:
		err = obj.client.WaitUntilInstanceTerminatedWithContext(ctx, waitInput)
	default:
		return false, errwrap.Wrapf(err, "unrecognized instance state")
	}
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == request.CanceledErrorCode {
				return false, errwrap.Wrapf(err, "waiter context cancelled")
			}
		}
		return false, errwrap.Wrapf(err, "waiter error")
	}

	// update the metadata before we return for send/recv
	instance, err = describeInstanceByID(obj.client, instance.InstanceId)
	if err != nil {
		return false, errwrap.Wrapf(err, "error descrining instance")
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *AwsEc2Res) Cmp(r engine.Res) error {
	// we can only compare AwsEc2Res to others of the same resource kind
	res, ok := r.(*AwsEc2Res)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Region != res.Region {
		return fmt.Errorf("the Region differs")
	}
	if obj.Type != res.Type {
		return fmt.Errorf("the Type differs")
	}
	if obj.ImageID != res.ImageID {
		return fmt.Errorf("the ImageID differs")
	}
	if obj.WatchEndpoint != res.WatchEndpoint {
		return fmt.Errorf("the WatchEndpoint differs")
	}
	if obj.WatchListenAddr != res.WatchListenAddr {
		return fmt.Errorf("the WatchListenAddr differs")
	}
	if obj.ErrorOnMalformedPost != res.ErrorOnMalformedPost {
		return fmt.Errorf("the ErrorOnMalformedPost differs")
	}
	if obj.UserData != res.UserData {
		return fmt.Errorf("the UserData differs")
	}
	return nil
}

func (obj *AwsEc2Res) prependName() string {
	return AwsPrefix + obj.Name()
}

// AwsEc2UID is the UID struct for AwsEc2Res.
type AwsEc2UID struct {
	engine.BaseUID

	name string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *AwsEc2Res) UIDs() []engine.ResUID {
	x := &AwsEc2UID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
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

// snsListener returns a listener bound to listenAddr.
func (obj *AwsEc2Res) snsListener(listenAddr string) (net.Listener, error) {
	addr := listenAddr
	// if listenAddr is a port
	if _, err := strconv.Atoi(listenAddr); err == nil {
		addr = fmt.Sprintf(":%s", listenAddr)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return listener, nil
}

// snsPostHandler listens for posts on the SNS Endpoint.
func (obj *AwsEc2Res) snsPostHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	// Decode the post. If an error is produced we either ignore the post,
	// or if ErrorOnMalformedPost is true, send the error through awsChan so
	// Watch() can return the error and the engine can shut down.
	decoder := json.NewDecoder(req.Body)
	var post postData
	if err := decoder.Decode(&post); err != nil {
		obj.init.Logf("error decoding post: %s", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		if obj.ErrorOnMalformedPost {
			select {
			case obj.awsChan <- &chanStruct{
				err: errwrap.Wrapf(err, "error decoding incoming POST, check struct formatting"),
			}:
			case <-obj.closeChan:
			}
		}
		return
	}
	// Verify the x509 signature. If there is an error verifying the
	// signature, we print the error, ignore the event and return.
	if err := obj.snsVerifySignature(post); err != nil {
		obj.init.Logf("error verifying signature: %s", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	// confirm the subscription
	if post.Type == "SubscriptionConfirmation" {
		if err := obj.snsConfirmSubscription(obj.snsTopicArn, post.Token); err != nil {
			select {
			case obj.awsChan <- &chanStruct{
				err: errwrap.Wrapf(err, "error confirming subscription"),
			}:
			case <-obj.closeChan:
			}
			return
		}
		// Now that the subscription is confirmed, we can tell the
		// engine we're running. If there is a delay between making the
		// request and the subscription actually being confirmed,
		// amazon will retry sending any new messages every 20 seconds
		// for one minute. So, we won't miss any events. See the
		// following for more details:
		// http://docs.aws.amazon.com/sns/latest/dg/SendMessageToHttp.html#SendMessageToHttp.retry
		select {
		case obj.awsChan <- &chanStruct{
			event: awsEc2EventWatchReady,
		}:
		case <-obj.closeChan:
		}
	}
	// process cloudwatch event notifications.
	if post.Type == "Notification" {
		event, err := obj.snsProcessEvent(post.Message, obj.prependName())
		if err != nil {
			select {
			case obj.awsChan <- &chanStruct{
				err: errwrap.Wrapf(err, "error processing event"),
			}:
			case <-obj.closeChan:
			}
			return
		}
		if event == awsEc2EventNone {
			return
		}
		select {
		case obj.awsChan <- &chanStruct{
			event: event,
		}:
		case <-obj.closeChan:
		}
	}
}

// snsVerifySignature verifies that the post messages are genuine and originate
// from amazon by checking if the signature is valid for the provided key and
// message contents.
func (obj *AwsEc2Res) snsVerifySignature(post postData) error {
	// download and parse the signing certificate
	cert, err := obj.snsGetCert(post.SigningCertURL)
	if err != nil {
		return errwrap.Wrapf(err, "error getting certificate")
	}
	// convert the message to canonical form
	message := obj.snsCanonicalFormat(post)
	// decode the message signature from base64
	signature, err := base64.StdEncoding.DecodeString(post.Signature)
	if err != nil {
		return errwrap.Wrapf(err, "error decoding string")
	}
	// check the signature against the message
	if err := cert.CheckSignature(x509.SHA1WithRSA, message, signature); err != nil {
		return errwrap.Wrapf(err, "error checking signature")
	}
	return nil
}

// snsGetCert downloads and parses the signing certificate from the provided URL
// for message verification.
func (obj *AwsEc2Res) snsGetCert(url string) (*x509.Certificate, error) {
	// only download valid certificates from amazon
	matchURL, err := regexp.MatchString(SnsCertURLRegex, url)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error matching regex")
	}
	if !matchURL {
		return nil, fmt.Errorf("invalid certificate url: %s", url)
	}
	// download the signing certificate
	resp, err := http.Get(url)
	if err != nil {
		return nil, errwrap.Wrapf(err, "http get error")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error reading post body")
	}
	// Decode the certificate and discard the second argument, which
	// contains any additional data in the response following the pem
	// block, if present.
	decodedCert, _ := pem.Decode(body)
	if decodedCert == nil {
		return nil, fmt.Errorf("certificate is nil")
	}
	// parse the certificate
	parsedCert, err := x509.ParseCertificate(decodedCert.Bytes)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error parsing certificate")
	}
	return parsedCert, nil
}

// snsCanonicalFormat formats post messages as required for signature
// verification. For more information about this requirement see:
// http://docs.aws.amazon.com/sns/latest/dg/SendMessageToHttp.verify.signature.html
func (obj *AwsEc2Res) snsCanonicalFormat(post postData) []byte {
	var str string
	str += "Message\n"
	str += post.Message + "\n"
	str += "MessageId\n"
	str += post.MessageID + "\n"
	if post.SubscribeURL != "" {
		str += "SubscribeURL\n"
		str += post.SubscribeURL + "\n"
	}
	str += "Timestamp\n"
	str += post.Timestamp + "\n"
	if post.Token != "" {
		str += "Token\n"
		str += post.Token + "\n"
	}
	str += "TopicArn\n"
	str += post.TopicArn + "\n"
	str += "Type\n"
	str += post.Type + "\n"

	return []byte(str)
}

// snsMakeTopic creates a topic on aws sns.
func (obj *AwsEc2Res) snsMakeTopic() (string, error) {
	// make topic
	topicInput := &sns.CreateTopicInput{
		Name: aws.String(SnsTopicName),
	}
	topic, err := obj.snsClient.CreateTopic(topicInput)
	if err != nil {
		return "", err
	}
	obj.init.Logf("Created SNS Topic")
	if topic.TopicArn == nil {
		return "", fmt.Errorf("the TopicArn is nil")
	}
	return *topic.TopicArn, nil
}

// snsDeleteTopic deletes the sns topic.
func (obj *AwsEc2Res) snsDeleteTopic(topicArn string) error {
	// delete the topic
	dtInput := &sns.DeleteTopicInput{
		TopicArn: aws.String(topicArn),
	}
	if _, err := obj.snsClient.DeleteTopic(dtInput); err != nil {
		return err
	}
	obj.init.Logf("Deleted SNS Topic")
	return nil
}

// snsSubscribe subscribes the endpoint to the sns topic. Returning
// SubscriptionArn here is useless as it is still pending confirmation.
func (obj *AwsEc2Res) snsSubscribe(endpoint string, topicArn string) error {
	// subscribe to the topic
	subInput := &sns.SubscribeInput{
		Endpoint: aws.String(endpoint),
		Protocol: aws.String(SnsSubscriptionProto),
		TopicArn: aws.String(topicArn),
	}
	_, err := obj.snsClient.Subscribe(subInput)
	if err != nil {
		return err
	}
	obj.init.Logf("Created Subscription")
	return nil
}

// snsConfirmSubscription confirms the sns subscription. Returning
// SubscriptionArn here is useless as it is still pending confirmation.
func (obj *AwsEc2Res) snsConfirmSubscription(topicArn string, token string) error {
	// confirm the subscription
	csInput := &sns.ConfirmSubscriptionInput{
		Token:    aws.String(token),
		TopicArn: aws.String(topicArn),
	}
	_, err := obj.snsClient.ConfirmSubscription(csInput)
	if err != nil {
		return err
	}
	obj.init.Logf("Subscription Confirmed")
	return nil
}

// snsProcessEvents unmarshals instance state-change notifications and, if the
// event matches the instance we are watching, returns an awsEc2Event.
func (obj *AwsEc2Res) snsProcessEvent(message, instanceName string) (awsEc2Event, error) {
	// unmarshal the message
	var msg postMsg
	if err := json.Unmarshal([]byte(message), &msg); err != nil {
		return awsEc2EventNone, err
	}
	// check if the instance id in the message matches the name of the
	// instance we're watching
	diInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(msg.InstanceID)},
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(instanceName)},
			},
		},
	}
	diOutput, err := obj.client.DescribeInstances(diInput)
	if err != nil {
		return awsEc2EventNone, err
	}
	// return the appropriate awsEc2Event
	if len(diOutput.Reservations) != 0 {
		switch msg.State {
		case "running":
			return awsEc2EventInstanceRunning, nil
		case "stopped":
			return awsEc2EventInstanceStopped, nil
		case "terminated":
			return awsEc2EventInstanceTerminated, nil
		}
	}
	return awsEc2EventNone, nil
}

// snsAuthorize adds the necessary permission for cloudwatch to publish to the
// SNS topic.
func (obj *AwsEc2Res) snsAuthorizeCloudWatch(topicArn string) error {
	// get the topic attributes, including the security policy
	gaInput := &sns.GetTopicAttributesInput{
		TopicArn: aws.String(topicArn),
	}
	attrs, err := obj.snsClient.GetTopicAttributes(gaInput)
	if err != nil {
		return err
	}
	// get the existing security policy
	pol := attrs.Attributes[SnsPolicy]
	// unmarshal the current sns security policy
	var policy snsPolicy
	if err := json.Unmarshal([]byte(*pol), &policy); err != nil {
		return err
	}
	// make sure the existing policy statement(s) are returned
	if policy.Statement == nil {
		return fmt.Errorf("sns policy statement is nil")
	}
	// construct a policy statement
	permission := snsStatement{
		Sid:    SnsPolicySid,
		Effect: SnsPolicyEffect,
		Principal: snsPrincipal{
			Service: SnsPolicyService,
		},
		Action:   SnsPolicyAction,
		Resource: topicArn,
	}
	// check if permissions have already been added
	for _, statement := range policy.Statement {
		if statement == permission {
			// if it's already there, we're done
			obj.init.Logf("Target Already Authorized")
			return nil
		}
	}
	// add the new policy statement to the existing one(s)
	policy.Statement = append(policy.Statement, permission)
	// marshal the updated policy
	newPolicyBytes, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	// update topic attributes with the new policy
	newPolicy := string(newPolicyBytes)
	saInput := &sns.SetTopicAttributesInput{
		AttributeName:  aws.String(SnsPolicy),
		AttributeValue: aws.String(newPolicy),
		TopicArn:       aws.String(topicArn),
	}
	_, err = obj.snsClient.SetTopicAttributes(saInput)
	if err != nil {
		return err
	}
	obj.init.Logf("Authorized Target")
	return nil
}

// cweMakeEventPattern makes and encodes event patterns for cloudwatch rules.
func (obj *AwsEc2Res) cweMakeEventPattern(source, detailType string, detail []string) (string, error) {
	pattern := cloudWatchRule{
		Source:     []string{source},
		DetailType: []string{detailType},
		Detail: ruleDetail{
			State: detail,
		},
	}
	eventPattern, err := json.Marshal(pattern)
	if err != nil {
		return "", err
	}
	return string(eventPattern), nil
}

// cweMakeRule makes a cloud watch rule.
func (obj *AwsEc2Res) cweMakeRule(name, eventPattern string) error {
	// make cloudwatch rule
	putRuleInput := &cwe.PutRuleInput{
		Name:         aws.String(name),
		EventPattern: aws.String(eventPattern),
	}
	if _, err := obj.cweClient.PutRule(putRuleInput); err != nil {
		return err
	}
	obj.init.Logf("Created CloudWatch Rule")
	return nil
}

// cweDeleteRule deletes the cloudwatch rule.
func (obj *AwsEc2Res) cweDeleteRule(name string) error {
	// delete the rule
	drInput := &cwe.DeleteRuleInput{
		Name: aws.String(name),
	}
	obj.init.Logf("Deleting CloudWatch Rule")
	if _, err := obj.cweClient.DeleteRule(drInput); err != nil {
		return errwrap.Wrapf(err, "error deleting cloudwatch rule")
	}
	return nil
}

// cweTargetRule configures cloudwatch to send events to sns topic.
func (obj *AwsEc2Res) cweTargetRule(topicArn, targetID, inputPath, ruleName string) error {
	// target the rule to sns topic
	target := &cwe.Target{
		Arn:       aws.String(topicArn),
		Id:        aws.String(targetID),
		InputPath: aws.String(inputPath),
	}
	putTargetInput := &cwe.PutTargetsInput{
		Rule:    aws.String(ruleName),
		Targets: []*cwe.Target{target},
	}
	_, err := obj.cweClient.PutTargets(putTargetInput)
	if err != nil {
		return errwrap.Wrapf(err, "error putting cloudwatch target")
	}
	obj.init.Logf("Targeted SNS Topic")
	return nil
}

// cweRemoveTarget removes the sns target from the cloudwatch rule.
func (obj *AwsEc2Res) cweRemoveTarget(targetID, ruleName string) error {
	// remove the target
	rtInput := &cwe.RemoveTargetsInput{
		Ids:  []*string{aws.String(targetID)},
		Rule: aws.String(ruleName),
	}
	obj.init.Logf("Removing Target")
	if _, err := obj.cweClient.RemoveTargets(rtInput); err != nil {
		return errwrap.Wrapf(err, "error removing cloudwatch target")
	}
	return nil
}

// stateWaiter waits for an instance to change state and returns the new state.
func stateWaiter(ctx context.Context, instance *ec2.Instance, c *ec2.EC2) (string, error) {
	var err error
	var name string

	// these cases are not permitted
	if instance == nil {
		return "", fmt.Errorf("nil instance")
	}
	if aws.StringValue(instance.State.Name) == "" {
		return "", fmt.Errorf("nil or empty state")
	}

	// get the instance name
	for _, tag := range instance.Tags {
		if aws.StringValue(tag.Key) == nameKey {
			name = aws.StringValue(tag.Value)
		}
	}
	// error if we didn't find one
	if name == "" {
		return "", fmt.Errorf("name not found")
	}

	// build the input for the waiters
	waitInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{instance.InstanceId},
		Filters: []*ec2.Filter{
			{
				Name:   aws.String(nameTag),
				Values: []*string{aws.String(name)},
			},
		},
	}
	// When we are watching terminated instances and waiting for them to exist,
	// we must exclude terminated instances from the waiter input. If we don't,
	// the waiter will return even if it finds a terminated instance, which is
	// not what we want.
	existWaiterFilter := &ec2.Filter{
		Name: aws.String("instance-state-name"),
		Values: []*string{
			aws.String(ec2.InstanceStateNameRunning),
			aws.String(ec2.InstanceStateNameStopped),
		},
	}
	// Select the appropriate waiter based on the instance state. There are
	// five possible states and we will catch every pertinent state change
	// (excluding transitional states) by waiting for the next state in the
	// instance's lifecycle. For more information about the lifecycle, see:
	// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-lifecycle.html
	switch aws.StringValue(instance.State.Name) {
	case ec2.InstanceStateNameRunning, ec2.InstanceStateNameStopping:
		err = c.WaitUntilInstanceStoppedWithContext(ctx, waitInput)
	case ec2.InstanceStateNameStopped, ec2.InstanceStateNamePending:
		err = c.WaitUntilInstanceRunningWithContext(ctx, waitInput)
	case ec2.InstanceStateNameTerminated:
		waitInput.Filters = append(waitInput.Filters, existWaiterFilter)
		err = c.WaitUntilInstanceExistsWithContext(ctx, waitInput)
	default:
		return "", fmt.Errorf("unrecognized instance state: %s", aws.StringValue(instance.State.Name))
	}
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if !ok {
			return "", errwrap.Wrapf(err, "error casting awserr")
		}
		// ignore these errors
		if aerr.Code() != request.CanceledErrorCode && aerr.Code() != request.WaiterResourceNotReadyErrorCode {
			return "", errwrap.Wrapf(err, "internal waiter error")
		}
		// If the waiter returns, because it has exceeded the maximum number of
		// attempts we return an empty state, which the event processing loop
		// ignores, and the longpollWatch goroutine will loop and restart
		// the waiter.
		if aerr.Message() == AwsErrExceededWaitAttempts {
			return "", nil
		}
	}

	// return the instance state
	instance, err = describeInstanceByName(c, name)
	if err != nil {
		return "", errwrap.Wrapf(err, "error describing instances")
	}
	return aws.StringValue(instance.State.Name), nil
}

// describeInstanceByName takes an ec2 client session and an instance name, and
// returns a *ec2.Instance or an error.
func describeInstanceByName(c *ec2.EC2, name string) (*ec2.Instance, error) {
	// get any instance with the specified name, that isn't terminated.
	diInput := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String(nameTag),
				Values: []*string{aws.String(name)},
			},
			{
				Name: aws.String("instance-state-name"),
				Values: []*string{
					aws.String(ec2.InstanceStateNameRunning),
					aws.String(ec2.InstanceStateNamePending),
					aws.String(ec2.InstanceStateNameStopped),
					aws.String(ec2.InstanceStateNameStopping),
				},
			},
		},
	}
	diOutput, err := c.DescribeInstances(diInput)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error describing instances")
	}

	// error if we get more than one reservation.
	if len(diOutput.Reservations) > 1 {
		return nil, fmt.Errorf("too many reservations")
	}
	// error if we got a reservation without exactly one instance.
	if len(diOutput.Reservations) != 0 && len(diOutput.Reservations[0].Instances) != 1 {
		return nil, fmt.Errorf("wrong number of instances")
	}

	// if we didn't find an instance, we consider it 'terminated'.
	if len(diOutput.Reservations) == 0 {
		return &ec2.Instance{
			State: &ec2.InstanceState{
				Name: aws.String(ec2.InstanceStateNameTerminated),
			},
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(nameKey),
					Value: aws.String(name),
				},
			},
		}, nil
	}

	return diOutput.Reservations[0].Instances[0], nil
}

// describeInstanceByID takes an ec2 client session and a pointer to an
// instanceID, and returns an *ec2.Instance or an error.
func describeInstanceByID(c *ec2.EC2, instanceID *string) (*ec2.Instance, error) {
	if instanceID == nil {
		return nil, fmt.Errorf("instanceID is nil")
	}

	// get any instance with the specified instanceID.
	diInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{instanceID},
	}
	diOutput, err := c.DescribeInstances(diInput)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error describing instances")
	}

	// error if we didn't find exactly one reservation with one instance.
	if len(diOutput.Reservations) != 1 {
		return nil, fmt.Errorf("wrong number of reservations")
	}
	if len(diOutput.Reservations[0].Instances) != 1 {
		return nil, fmt.Errorf("wrong number of instances")
	}

	return diOutput.Reservations[0].Instances[0], nil
}
