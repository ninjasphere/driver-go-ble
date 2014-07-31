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

type waypointPayload struct {
	Sequence    uint8
	AddressType uint8
	Rssi        int8
	Valid       uint8
}

// configure the agent logger
var log = logger.GetLogger("driver-go-ble")

/*  var packet = {
      device: this.vars.ieee,
      waypoint: nobleieeeToIEEE(peripheral.uuid),
      rssi: this.vars.rssi
    };

    self.locatorDevice.sendEvent('locator', 'advertisement', packet);

    // XXX: Temporary remove me
    self.bus.publish('$device/' + packet.device.replace(/[:\r\n]/g, '') + '/TEMPPATH/rssi', packet);*/

func sendRssi(device string, waypoint string, rssi int8, conn *ninja.NinjaConnection) {
	log.Infof(">> Device:%s Waypoint:%s Rssi: %d", device, waypoint, rssi)

	packet, _ := simplejson.NewJson([]byte(`{
			"device": "",
			"waypoint": "",
			"rssi": 0
	}`))
	packet.Set("device", device)
	packet.Set("waypoint", waypoint)
	packet.Set("rssi", rssi)

	conn.PublishMessage("$device/"+device+"/TEMPPATH/rssi", packet)
}

func main() {
	os.Exit(realMain())
}

func realMain() int {

	// configure the agent logger
	log := logger.GetLogger("driver-ble")

	// main logic here
	var conn, err = ninja.Connect("com.ninjablocks.ble")

	if err != nil {
		log.Errorf("Connect failed: %v", err)
		return 1
	}

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
		/*Rssi: func(address string, rssi int8) {
		  log.Printf("Rssi update address:%s rssi:%d", address, rssi)
		  //spew.Dump(device);
		},*/
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

	client.Rssi = func(address string, rssi int8) {
		//log.Printf("Rssi update address:%s rssi:%d", address, rssi)
		sendRssi(strings.Replace(address, ":", "", -1), mac, rssi, conn)
		//spew.Dump(device);
	}

	client.Discover = func(device *gatt.DiscoveredDevice) {
		log.Debugf("Discovered address:%s rssi:%d", device.Address, device.Rssi)

		if device.Advertisement.LocalName != "NinjaSphereWaypoint" {
			return
		}

		err := client.Connect(device.Address, device.PublicAddress)
		if err != nil {
			log.Errorf("Connect error:%s", err)
		}

		device.Connected = func() {
			log.Infof("Connected to waypoint: %s", device.Address)
			//spew.Dump(device.Advertisement)

			// XXX: Yes, magic numbers.... this enables the notification from our Waypoints
			client.Notify(device.Address, true, 45, 48, true, false)
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

			//spew.Dump("ieee:", reverse(notification.Data[4:]), strings.ToUpper(ieee.String()), payload)

			sendRssi(fmt.Sprintf("%x", reverse(notification.Data[4:])), strings.Replace(device.Address, ":", "", -1), payload.Rssi, conn)
		}

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