package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	"github.com/ninjasphere/gatt"
	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/logger"
)

const driverName = "driver-ble"

type waypointPayload struct {
	Sequence    uint8
	AddressType uint8
	Rssi        int8
	Valid       uint8
}

type adPacket struct {
	Device   string `json:"device"`
	Waypoint string `json:"waypoint"`
	Rssi     int8   `json:"rssi"`
	IsSphere bool   `json:"isSphere"`
}

type ninjaPacket struct {
	Device   string `json:"device"`
	Waypoint string `json:"waypoint"`
	Rssi     int8   `json:"rssi"`
	IsSphere bool   `json:"isSphere"`
	name 	 string `json:"name,omitempty"`
}

// configure the agent logger
var log = logger.GetLogger("driver-go-ble")

//var mesh *udpMesh

func sendRssi(device string, name string, waypoint string, rssi int8, isSphere bool, conn *ninja.Connection) {
	device = strings.ToUpper(device)

	log.Debugf(">> Device:%s Waypoint:%s Rssi: %d", device, waypoint, rssi)

	ninjaPacket := ninjaPacket{
		Device:   device,
		Waypoint: waypoint,
		Rssi:     rssi,
		IsSphere: isSphere,
		name: name,
	}

	//spew.Dump(packet)
	conn.SendNotification("$device/"+device+"/TEMPPATH/rssi", ninjaPacket)

}

func publishMessage(conn *ninja.Connection, topic string, packet interface{}) {
	p, err := json.Marshal(packet)
	if err == nil {
		conn.GetMqttClient().Publish(mqtt.QoS(0), topic, p)
	} else {
		log.Fatalf("marshalling error for %v", packet)
	}
}

func main() {
	os.Exit(realMain())
}

func realMain() int {

	log.Infof("Starting " + driverName)

	conn, err := ninja.Connect("com.ninjablocks.ble")

	if err != nil {
		log.FatalErrorf(err, "Could not connect to MQTT Broker")
	}

	/*if mesh, err = newUdpMesh("239.255.12.34:12345", func(packet *adPacket) {

		spew.Dump("Got mesh packet", packet)

	}); err != nil {
		log.FatalErrorf(err, "Could not connect to UDP mesh")
	}*/

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

	go func() {
		for {
			time.Sleep(time.Second)
			waypoints := 0
			for id, active := range activeWaypoints {
				log.Debugf("Waypoint %s is active? %t", id, active)
				if active {
					waypoints++
				}
			}
			log.Debugf("%d waypoint(s) active", waypoints)

			publishMessage(conn, "$location/waypoints", waypoints)
		}
	}()

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

				packet := &adPacket{
					Device:   fmt.Sprintf("%x", reverse(notification.Data[4:])),
					Waypoint: strings.Replace(device.Address, ":", "", -1),
					Rssi:     payload.Rssi,
					IsSphere: false,
				}

				sendRssi(packet.Device, "", packet.Waypoint, packet.Rssi, packet.IsSphere, conn)
				//mesh.send(packet)
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
		log.FatalError(err, "Failed to start client")
	}

	err = client.StartScanning(true)
	if err != nil {
		log.FatalError(err, "Failed to start scanning")
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
