package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/teslamotors/vehicle-command/pkg/connector/ble"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
	"github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/universalmessage"
	"github.com/teslamotors/vehicle-command/pkg/vehicle"
	"golang.org/x/exp/maps"
)

var _conn *ble.Connection = nil
var _vehicle *vehicle.Vehicle = nil
var _connected = false
var _tmrClean *time.Timer = nil
var _tmrCleanSecs = 10

func nullifyConnVehicle() {
	if _conn != nil {
		defer _conn.Close()
	}
	if _vehicle != nil {
		defer _vehicle.Disconnect()
	}
	_conn = nil
	_vehicle = nil
	_connected = false
	_tmrClean.Stop()
}

func wakeup() (bool, error) {
	//Wake up vehicle
	ctx, cancel := context.WithTimeout(context.Background(), _timeout)
	defer cancel()

	fmt.Println("Waking up vehicele")

	if err := _vehicle.StartSession(ctx, []universalmessage.Domain{protocol.DomainVCSEC}); err != nil {
		fmt.Printf("Failed to perform handshake with vehicle: %s\n", err)
		nullifyConnVehicle()
		return false, err
	}
	err := commands["wake"].handler(ctx, nil, _vehicle, nil)
	if err != nil {
		fmt.Printf("Failed to send wake command to vehicle: %s\n", err)
		nullifyConnVehicle()
		return false, err
	}
	fmt.Println("Woke up vehicele")

	return true, nil
}
func connectToCar() (bool, error) {
	if _connected {
		_tmrClean.Reset(time.Duration(_tmrCleanSecs) * time.Second)
		return true, nil
	}

	if _tmrClean == nil {
		_tmrClean = time.AfterFunc(2*time.Second, func() {
			fmt.Println("Disconnecting from car...")
			nullifyConnVehicle()
		})

	}
	_tmrClean.Stop()

	var err error = nil

	ctx, cancel := context.WithTimeout(context.Background(), _timeout)
	defer cancel()

	fmt.Println("Creating connection...")
	_conn, err = ble.NewConnection(ctx, _vin)
	if err != nil {
		fmt.Printf("Failed to connect to vehicle: %s", err)
		nullifyConnVehicle()
		return false, err
	}
	// defer _conn.Close()

	fmt.Println("New Vehicle costructor")
	_vehicle, err = vehicle.NewVehicle(_conn, _privKey, nil)
	if err != nil {
		fmt.Printf("Failed to connect to vehicle: %s", err)
		nullifyConnVehicle()
		return false, err
	}

	fmt.Println("Connecting to car...")
	if err = _vehicle.Connect(ctx); err != nil {
		fmt.Printf("Failed to connect to vehicle: %s\n", err)

		nullifyConnVehicle()
		return false, err

	}

	if _privKeyPath != "" {
		fmt.Println("Starting session to car...")
		if err = _vehicle.StartSession(ctx, nil); err != nil {

			// If the vehicle is sleeping wakes it up
			if strings.Contains(err.Error(), "context deadline exceeded") {
				var ok bool
				ok, err = wakeup()
				if ok {
					ctx, cancel := context.WithTimeout(context.Background(), _timeout)
					defer cancel()
					if err = _vehicle.StartSession(ctx, nil); err != nil {
						fmt.Printf("Failed to perform handshake with vehicle after wake up: %s\n", err)
						nullifyConnVehicle()
						return false, err
					}
				} else {
					fmt.Printf("Failed to wake up vehicle: %s\n", err)
					nullifyConnVehicle()
					return false, err
				}

			} else {
				fmt.Printf("Failed to perform handshake with vehicle: %s\n", err)
				nullifyConnVehicle()
				return false, err
			}

		}

	}

	fmt.Println("Car connected")
	_tmrClean.Reset(time.Duration(_tmrCleanSecs) * time.Second)
	_connected = true
	return true, nil
}

func printCommands(w http.ResponseWriter) {
	if _privKey == nil {
		io.WriteString(w, "WARNING: No private key set, the available commands will be very limited\n\n")
	}

	io.WriteString(w, "List of supported commands\n")
	keys := maps.Keys(commands)
	slices.Sort(keys)
	for _, key := range keys {
		value := commands[key]
		if !value.requiresFleetAPI {
			if !value.requiresAuth || _privKey != nil {
				io.WriteString(w, fmt.Sprintf("Name: %s\n%s\n", key, value.help))
				if len(value.args) > 0 {
					io.WriteString(w, "Arguments:\n")
					for _, arg := range value.args {
						io.WriteString(w, fmt.Sprintf("name: %s - Description: %s\n", arg.name, arg.help))
					}
				}
				io.WriteString(w, "\n")
			}
		}
	}

}
func handleCommand(w http.ResponseWriter, r *http.Request) {
	responseCode := http.StatusNotFound
	var logLines string

	defer func() {
		fmt.Printf("Request uri from %s: %s - Response code: %d\n", r.RemoteAddr, r.RequestURI, responseCode)
		if logLines != "" {
			fmt.Println(logLines)
		}
		if responseCode != http.StatusOK {
			w.WriteHeader(responseCode)
		}
	}()

	if r.URL.Path == "/" {
		responseCode = http.StatusOK
		printCommands(w)
		return
	}

	info, ok := commands[r.URL.Path[1:]]
	if !ok {
		return
	}
	if info.requiresFleetAPI {
		w.Header().Set("x-bad-request-field", fmt.Sprintf("Method %s is only supported via Fleet API", r.URL.Path[1:]))
		responseCode = http.StatusBadRequest
		return
	}
	if info.requiresAuth && _privKey == nil {
		w.Header().Set("x-bad-request-field", fmt.Sprintf("Method %s is only supported via Authentication", r.URL.Path[1:]))
		responseCode = http.StatusBadRequest
		return
	}

	logLines = fmt.Sprintf("%s\n%s\n%s", info.help, r.URL.Path[1:], r.URL.Query())

	if r.URL.Query().Has("help") {
		io.WriteString(w, fmt.Sprintln(info.help))
		for _, value := range info.args {
			io.WriteString(w, fmt.Sprintf("Argument name: %s - Description: %s\n", value.name, value.help))
		}
		responseCode = http.StatusOK
		return
	}

	// Set arguments
	args := make(map[string]string)

	for _, value := range info.args {
		if !r.URL.Query().Has(value.name) {
			w.Header().Set("x-bad-request-field", fmt.Sprintf("Missing %s parameter. use help query string for a complete list of arguments", value.name))
			responseCode = http.StatusBadRequest
			return
		} else {
			args[value.name] = r.URL.Query().Get(value.name)
		}
	}

	ok, err := connectToCar()
	if ok {
		responseCode = http.StatusOK
		ctx, cancel := context.WithTimeout(context.Background(), _timeout)
		defer cancel()
		err = info.handler(ctx, nil, _vehicle, args)
		if err != nil {
			w.Header().Set("x-error", fmt.Sprintf("Error: %s", err))
			responseCode = http.StatusInternalServerError
			return
		}

	} else {
		fmt.Printf("Error connecting to car: %s\n", err)
		w.Header().Set("x-error", fmt.Sprintf("Error connecting to car: %s", err))
		responseCode = http.StatusInternalServerError
		return
	}

}
