// This is an example mock BMC server/device.
// Many thanks to Joel Rebello for figuring out the specific endpoints needed.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/purpleidea/mgmt/util"

	"github.com/alexflint/go-arg"
	"github.com/bmc-toolbox/bmclib/v2/providers/rpc"
)

const (
	// DefaultPort to listen on. Seen in the bmclib docs.
	DefaultPort = 8800

	// StateOn is the power "on" state.
	StateOn = "on"

	// StateOff is the power "off" state.
	StateOff = "off"
)

// MockBMC is a simple mocked BMC device.
type MockBMC struct {
	// Addr to listen on. Eg: :8800 for example.
	Addr string

	// State of the device power. This gets read and changed by the API.
	State string

	// Driver specifies which driver we want to mock.
	// TODO: Do I mean "driver" or "provider" ?
	Driver string
}

// Data is what we use to template the outputs.
type Data struct {
	PowerState string
}

// Run kicks this all off.
func (obj *MockBMC) Run() error {

	tls := util.NewTLS()
	tls.Host = "localhost" // TODO: choose something
	keyPemFile := "/tmp/key.pem"
	certPemFile := "/tmp/cert.pem"

	if err := tls.Generate(keyPemFile, certPemFile); err != nil {
		return err
	}

	fmt.Printf("running at: %s\n", obj.Addr)
	fmt.Printf("driver is: %s\n", obj.Driver)
	fmt.Printf("device is: %s\n", obj.State) // we start off in this state
	if obj.Driver == "rpc" {
		http.HandleFunc("/", obj.rpcHandler)
	}
	if obj.Driver == "redfish" || obj.Driver == "gofish" {
		http.HandleFunc("/redfish/v1/", obj.endpointFunc("service_root.json", http.MethodGet, 200, nil))

		// login
		sessionHeader := map[string]string{
			"X-Auth-Token": "t5tpiajo89fyvvel5434h9p2l3j69hzx", // TODO: how do we get this?
			"Location":     "/redfish/v1/SessionService/Sessions/1",
		}

		http.HandleFunc("/redfish/v1/SessionService/Sessions", obj.endpointFunc("session_service.json", http.MethodPost, 201, sessionHeader))

		// get power state
		http.HandleFunc("/redfish/v1/Systems", obj.endpointFunc("systems.json", http.MethodGet, 200, nil))
		http.HandleFunc("/redfish/v1/Systems/1", obj.endpointFunc("systems_1.json.tmpl", http.MethodGet, 200, nil))

		// set pxe - we can't have two routes with the same pattern
		//http.HandleFunc("/redfish/v1/Systems/1", obj.endpointFunc("", http.MethodPatch, 200, nil))

		// power on/off XXX: seems to post here to turn on
		http.HandleFunc("/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", obj.endpointFunc("", http.MethodPost, 200, nil))

		// logoff
		http.HandleFunc("/redfish/v1/SessionService/Sessions/1", obj.endpointFunc("session_delete.json", http.MethodDelete, 200, nil))
	}

	http.HandleFunc("/hello", obj.hello)
	//return http.ListenAndServe(obj.Addr, nil)
	return http.ListenAndServeTLS(obj.Addr, certPemFile, keyPemFile, nil)
}

func (obj *MockBMC) template(templateText string, data interface{}) (string, error) {
	var err error
	tmpl := template.New("name") // whatever name you want
	//tmpl = tmpl.Funcs(funcMap)
	tmpl, err = tmpl.Parse(templateText)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)

	// run the template
	if err := tmpl.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// endpointFunc handles the bmc mock requirements.
func (obj *MockBMC) endpointFunc(file, method string, retStatus int, retHeader map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		data := &Data{
			PowerState: obj.State,
		}

		fmt.Printf("URL: %s\n", r.URL.Path)
		fmt.Printf("[%s] file: %s\n", method, file)
		//if method == "POST" {
		//for name, values := range r.Header {
		//	fmt.Printf("\t[%s] header: %+v\n", name, values)
		//}

		if err := r.ParseForm(); err != nil {
			fmt.Printf("error parsing form: %+v\n", err)
		}
		for name, values := range r.PostForm {
			fmt.Printf("\t[%s] values: %+v\n", name, values)
		}
		//}

		// purge check on patch method if set pxe request is attempted
		if r.Method != method && r.Method != http.MethodPatch {
			resp := fmt.Sprintf("unexpected request - url: %s, method: %s", r.URL, r.Method)
			_, _ = w.Write([]byte(resp))
		}

		for k, v := range retHeader {
			w.Header().Add(k, v)
		}

		w.WriteHeader(retStatus)
		if file != "" {
			out1 := mustReadFile(file)
			if !strings.HasSuffix(file, ".tmpl") {
				_, _ = w.Write(out1)
				return
			}

			out2, err := obj.template(string(out1), data)
			if err != nil {
				resp := fmt.Sprintf("unexpected request - url: %s, method: %s", r.URL, r.Method)
				_, _ = w.Write([]byte(resp))
				return
			}
			_, _ = w.Write([]byte(out2))

		}

		return
	}
}

// rpcHandler is used for the rpc driver.
func (obj *MockBMC) rpcHandler(w http.ResponseWriter, r *http.Request) {
	//fmt.Printf("req1: %+v\n", r)
	//fmt.Printf("method: %s\n", r.Method)
	//fmt.Printf("URL: %s\n", r.URL)
	//fmt.Printf("Body: %v\n", r.Body)

	req := rpc.RequestPayload{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	//fmt.Printf("data: %+v\n", req)

	rp := rpc.ResponsePayload{
		ID:   req.ID,
		Host: req.Host,
	}
	switch req.Method {
	case rpc.PowerGetMethod:
		rp.Result = obj.State
		fmt.Printf("get state: %s\n", obj.State)

	case rpc.PowerSetMethod:
		//fmt.Printf("req2: %T %+v\n", req.Params, req.Params)
		// TODO: This is a mess, isn't there a cleaner way to unpack it?
		m, ok := req.Params.(map[string]interface{})
		if ok {
			param, exists := m["state"]
			state, ok := param.(string)
			if ok {
				if exists && (state == StateOn || state == StateOff) {
					obj.State = state
					fmt.Printf("set state: %s\n", state)
				}
			}
		}

	case rpc.BootDeviceMethod:

	case rpc.PingMethod:
		fmt.Printf("got ping\n")
		rp.Result = "pong"

	default:
		w.WriteHeader(http.StatusNotFound)
	}
	b, _ := json.Marshal(rp)
	//fmt.Printf("out: %s\n", b)
	w.Write(b)
}

func (obj *MockBMC) hello(w http.ResponseWriter, req *http.Request) {
	fmt.Printf("req: %+v\n", req)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("This is hello world!\n"))
	//w.Write([]byte("OpenBMC says hello!\n"))
}

func mustReadFile(filename string) []byte {
	fixture := "fixtures" + "/" + filename
	fh, err := os.Open(fixture)
	if err != nil {
		panic(err)
	}

	defer fh.Close()

	b, err := io.ReadAll(fh)
	if err != nil {
		panic(err)
	}

	// The original examples had no trailing newlines. Not sure if allowed.
	return bytes.TrimSuffix(b, []byte("\n"))
}

// Args are what are used to build the CLI.
type Args struct {
	// XXX: We cannot have both subcommands and a positional argument.
	// XXX: I think it's a bug of this library that it can't handle argv[0].
	//Argv0 string `arg:"positional"`

	On bool `arg:"--on" help:"start on"`

	Port int `arg:"--port" help:"port to listen on"`

	Driver string `arg:"--driver" default:"redfish" help:"driver to use"`
}

// Main program that returns error.
func Main() error {
	args := Args{
		Port: DefaultPort,
	}
	config := arg.Config{}
	parser, err := arg.NewParser(config, &args)
	if err != nil {
		// programming error
		return err
	}
	err = parser.Parse(os.Args[1:]) // XXX: args[0] needs to be dropped
	if err == arg.ErrHelp {
		parser.WriteHelp(os.Stdout)
		return nil
	}

	state := StateOff
	if args.On {
		state = StateOn
	}

	mock := &MockBMC{
		Addr: fmt.Sprintf("localhost:%d", args.Port),
		//State: StateOff, // starts off off
		//State: StateOn,
		State:  state,
		Driver: args.Driver,
	}
	return mock.Run()
}

// wget --no-check-certificate --post-data 'user=foo&password=bar' \
// https://localhost:8800/redfish/v1/Systems/1/Actions/ComputerSystem.Reset -O -
func main() {
	fmt.Printf("main: %+v\n", Main())
}
