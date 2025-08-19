package main

import (
	"fmt"
	"lab1/shared"
	"net"
	"encoding/json"
)

func main() {
	// Porta fixa (10001)
	Address, err := net.ResolveUDPAddr("udp", "127.0.0.1:10001")
	shared.CheckError(err)
	Connection, err := net.ListenUDP("udp", Address)
	shared.CheckError(err)
	defer Connection.Close()

	buf := make([]byte, 1024)
	for {
		n, addr, err := Connection.ReadFromUDP(buf)
		shared.PrintError(err)

		var msg shared.Message
		err = json.Unmarshal(buf[:n], &msg)

		shared.PrintError(err)

		fmt.Println("Received PID: ", msg.PID, "-", msg.Text, " from ", addr, " at clock ", msg.MsgClock)

		

	}
}
