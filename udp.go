package main

import (
	"encoding/json"
	"net"
)

type udpMesh struct {
	conn     *net.UDPConn
	address  *net.UDPAddr
	onPacket func(packet *adPacket)
}

func newUdpMesh(udpHostAndPort string, onPacket func(packet *adPacket)) (udp *udpMesh, err error) {

	var udpaddr *net.UDPAddr
	if udpaddr, err = net.ResolveUDPAddr("udp4", udpHostAndPort); err != nil {
		return nil, err
	}

	var conn *net.UDPConn
	if conn, err = net.ListenUDP("udp4", udpaddr); err != nil {
		return nil, err
	}

	mesh := &udpMesh{
		conn:     conn,
		address:  udpaddr,
		onPacket: onPacket,
	}

	go mesh.start()

	return mesh, nil
}

func (m *udpMesh) send(packet *adPacket) error {

	payload, err := json.Marshal(packet)
	if err != nil {
		return err
	}

	_, err = m.conn.WriteToUDP(payload, m.address)

	return err
}

func (m *udpMesh) start() {

	for {
		buffer := make([]byte, 256)

		if c, addr, err := m.conn.ReadFromUDP(buffer); err != nil {

			log.Infof("ble mesh: %d byte datagram from %s with error %s\n", c, addr.String(), err.Error())

		} else {

			packet := &adPacket{}

			err := json.Unmarshal(buffer[:c], packet)

			if err != nil {
				log.Errorf("Invalid JSON: %s", err)
			} else {
				m.onPacket(packet)
			}
		}

	}

}
