package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/teslamotors/vehicle-command/pkg/protocol"
)

// Settings
var _timeout_str = os.Getenv("TESLA_SESSION_SECS")
var _timeout = 30 * time.Second

var _vin = os.Getenv("TESLA_VIN")
var _privKeyPath = os.Getenv("TESLA_KEY_FILE")
var _privKey protocol.ECDHPrivateKey = nil
var _listen_addr = os.Getenv("LISTEN_ADDRESS")

func getFavicon(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func writeErr(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {
	var err error

	if _listen_addr == "" {
		_listen_addr = ":3333"
	}

	if _timeout_str != "" {
		var timeout_int int
		timeout_int, err = strconv.Atoi(_timeout_str)
		if err == nil {
			_timeout = time.Duration(timeout_int) * time.Second
		}

	}

	if _vin == "" {
		fmt.Println("TESLA_VIN ENV Variable not set. Exiting")
		os.Exit(2)
	}
	if _privKeyPath != "" {
		if _privKey, err = protocol.LoadPrivateKey(_privKeyPath); err != nil {
			fmt.Printf("Failed to load private key: %s", err)
			os.Exit(3)
			return
		}
	} else {
		fmt.Println("WARNING: No private key set, the available commands will be very limited")
	}

	http.HandleFunc("/", handleCommand)
	http.HandleFunc("/favicon.ico", getFavicon)

	err = http.ListenAndServe(_listen_addr, nil)

	if err != nil {
		fmt.Printf("Error listening on address %s. Error: %s", _listen_addr, err)
		os.Exit(1)
		return
	} else {
		fmt.Printf("listening on address %s", _listen_addr)
	}

}
