package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"

	"github.com/bitly/go-simplejson"
	"github.com/ninjasphere/gatt"
	"github.com/ninjasphere/go-ninja"
	"github.com/ninjasphere/go-ninja/logger"
)

const driverName = "driver-ble"

type waypointPayload struct {
	Sequence    uint8
	AddressType uint8
	Rssi        int8
	Valid       uint8
}

// configure the agent logger
var log = logger.GetLogger("driver-go-ble")

func sendRssi(device string, name string, waypoint string, rssi int8, isSphere bool, conn *ninja.NinjaConnection) {
	log.Debugf(">> Device:%s Waypoint:%s Rssi: %d", device, waypoint, rssi)

	packet, _ := simplejson.NewJson([]byte(`{
    "params": [
        {
            "device": "",
            "waypoint": "",
            "rssi": 0,
            "isSphere": true
        },
        "locator"
    ],
    "time": 0,
    "jsonrpc": "2.0"
}`))

	packet.Get("params").GetIndex(0).Set("device", device)
	if name != "" {
		packet.Get("params").GetIndex(0).Set("name", name)
	}
	packet.Get("params").GetIndex(0).Set("waypoint", waypoint)
	packet.Get("params").GetIndex(0).Set("rssi", rssi)
	packet.Get("params").GetIndex(0).Set("isSphere", isSphere)

	//spew.Dump(packet)
	conn.PublishMessage("$device/"+device+"/TEMPPATH/rssi", packet)
}

func main() {
	os.Exit(realMain())
}

func realMain() int {

	log.Infof("Starting " + driverName)

	var conn, err = ninja.Connect("com.ninjablocks.ble")

	if err != nil {
		log.FatalErrorf(err, "Could not connect to MQTT Broker")
	}

	statusJob, err := ninja.CreateStatusJob(conn, driverName)

	if err != nil {
		log.FatalErrorf(err, "Could not setup status job")
	}

	statusJob.Start()

	out, err := exec.Command("hciconfig").Output()
	if err != nil {
		log.Errorf(fmt.Sprintf("Error: %s", err))
	}
	re := regexp.MustCompile("([0-9A-F]{2}\\:{0,1}){6}")
	mac := strings.Replace(re.FindString(string(out)), ":", "", -1)
	log.Infof("The local mac is %s\n", mac)

	client := &gatt.Client{
		StateChange: func(newState string) {
			log.Infof("Client state change: %s", newState)
		},
	}

	/*
	  Waypoint notification characteristic {
	    "startHandle": 45,
	    "properties": 16, (useNotify = true, useIndicate = false)
	    "valueHandle": 46,
	    "uuid": "fff4",
	    "endHandle": 48,
	  }
	*/

	activeWaypoints := make(map[string]bool)

	client.Rssi = func(address string, name string, rssi int8) {
		//log.Printf("Rssi update address:%s rssi:%d", address, rssi)
		sendRssi(strings.Replace(address, ":", "", -1), name, mac, rssi, true, conn)
		//spew.Dump(device);
	}

	client.Advertisement = func(device *gatt.DiscoveredDevice) {
		log.Debugf("Discovered address:%s rssi:%d", device.Address, device.Rssi)

		if device.Advertisement.LocalName != "NinjaSphereWaypoint" {
			return
		}

		if activeWaypoints[device.Address] {
			return
		}

		if device.Connected == nil {
			device.Connected = func() {
				log.Infof("Connected to waypoint: %s", device.Address)
				//spew.Dump(device.Advertisement)

				// XXX: Yes, magic numbers.... this enables the notification from our Waypoints
				client.Notify(device.Address, true, 45, 48, true, false)
			}

			device.Disconnected = func() {
				log.Infof("Disconnected from waypoint: %s", device.Address)

				activeWaypoints[device.Address] = false
			}

			device.Notification = func(notification *gatt.Notification) {
				//log.Printf("Got the notification!")

				//XXX: Add the ieee into the payload somehow??
				var payload waypointPayload
				err := binary.Read(bytes.NewReader(notification.Data), binary.LittleEndian, &payload)
				if err != nil {
					log.Errorf("Failed to read waypoint payload : %s", err)
				}

				//	ieee := net.HardwareAddr(reverse(notification.Data[4:]))

				//spew.Dump("ieee:", payload)

				sendRssi(fmt.Sprintf("%x", reverse(notification.Data[4:])), "", strings.Replace(device.Address, ":", "", -1), payload.Rssi, false, conn)
			}
		}

		err := client.Connect(device.Address, device.PublicAddress)
		if err != nil {
			log.Errorf("Connect error:%s", err)
			return
		}

		activeWaypoints[device.Address] = true

	}

	err = client.Start()

	if err != nil {
		log.Errorf("Failed to start client: %s", err)
	}

	err = client.StartScanning(true)
	if err != nil {
		log.Errorf("Failed to start scanning: %s", err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	// Block until a signal is received.
	s := <-c
	log.Infof("Got signal:", s)

	return 0
}

// reverse returns a reversed copy of u.
func reverse(u []byte) []byte {
	l := len(u)
	b := make([]byte, l)
	for i := 0; i < l/2+1; i++ {
		b[i], b[l-i-1] = u[l-i-1], u[i]
	}
	return b
}
