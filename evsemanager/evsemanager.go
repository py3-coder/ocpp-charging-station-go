package evsemanager

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type AsyncEVSEMessage struct {
	Message         string
	SuccessCallback func(string)
}

type EVSE struct {
	Id                               int
	IsEVConnected                    int
	IsChargingEnabled                int
	IsCharging                       int
	IsError                          int
	EnergyActiveNet_kwh_times100     int64
	PowerActiveImport_kw_times100    int64
	OnEVConnected_fire_once          func()
	OnEVDisconnected_fire_once       func()
	OnEVSEChargingEnabled_fire_once  func()
	OnEVSEChargingDisabled_fire_once func()
	OnEVSEChargingStarted_fire_once  func()
	OnEVSEChargingStopped_fire_once  func()
	OnEVSEError_fire_once            func()
	OnEVSENoError_fire_once          func()
	OnEVConnected_repeat             func()
	OnEVDisconnected_repeat          func()
	OnEVSEChargingEnabled_repeat     func()
	OnEVSEChargingDisabled_repeat    func()
	OnEVSEChargingStarted_repeat     func()
	OnEVSEChargingStopped_repeat     func()
	OnEVSEError_repeat               func()
	OnEVSENoError_repeat             func()
}

var tcp_conn *net.TCPConn
var in_channel chan string
var out_channel chan string
var messages_awaiting_resp chan AsyncEVSEMessage
var lastMessageSentAt time.Time
var status_strings chan string

func ConnectNewEVSE(id int, servAddr string) (*EVSE, error) {
	// Create new EVSE object
	evse := &EVSE{
		// Fill it up with default values
		Id:                            id,
		IsEVConnected:                 0,
		IsChargingEnabled:             0,
		IsCharging:                    0,
		IsError:                       0,
		EnergyActiveNet_kwh_times100:  0,
		PowerActiveImport_kw_times100: 0,
		OnEVConnected_repeat:          func() { fmt.Println("EVConnected - No override") },
		OnEVDisconnected_repeat:       func() { fmt.Println("EVDisconnected - No override") },
		OnEVSEChargingEnabled_repeat:  func() { fmt.Println("EVSEChargingEnabled - No override") },
		OnEVSEChargingDisabled_repeat: func() { fmt.Println("EVSEChargingDisabled - No override") },
		OnEVSEChargingStarted_repeat:  func() { fmt.Println("EVSEChargingStarted - No override") },
		OnEVSEChargingStopped_repeat:  func() { fmt.Println("EVSEChargingStopped - No override") },
		OnEVSEError_repeat:            func() { fmt.Println("EVSEError - No override") },
		OnEVSENoError_repeat:          func() { fmt.Println("EVSEError - No override") },
	}

	in_channel = make(chan string)
	out_channel = make(chan string)

	// Connect to EVSE TCP server
	tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
	if err != nil {
		println("ResolveTCPAddr failed:", err.Error())
		return nil, err
	}
	tcp_conn, err = net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		println("Dial failed:", err.Error())
		return nil, err
	}

	return evse, nil
}

func (evse *EVSE) Start() {

	go inLoop()
	go sendloop()

	// Start message inbox thread for this EVSE instance
	go runInbox()

	go evse.updatestatusloop()
	// Run the status polling
	go evse.statusPollLoop()
	// return new EVSE instance
}

func (evse *EVSE) updatestatusloop() {
	for status_s := range status_strings {
		evse.updateStatus(status_s)
	}
}

func (evse *EVSE) Disconnect() {
	tcp_conn.Close()
}

func runInbox() {
	reply := make([]byte, 1024)
	for {
		_, err := tcp_conn.Read(reply)
		if err != nil {
			println("TCP read failed:", err.Error())
			os.Exit(1)
		}
		println("reply from server=", string(reply))
		in_channel <- string(reply)

	}
}

func inLoop() {
	for message_string := range in_channel {
		// invoke callback
		message_awaiting := <-messages_awaiting_resp
		message_awaiting.SuccessCallback(message_string)
		// // empty the message holder
		// evse.message_awaiting_response = nil

	}
}

func (evse *EVSE) updateStatus(statusString string) {
	fmt.Println("Original status string: ", statusString)
	split_result := strings.Split(statusString, ",")
	if len(split_result) != 4 {
		log.Error("Unable to update status, status string is length is not 4")
		return
	}

	fmt.Println("split_result[0]", split_result[0])
	fmt.Println("split_result[1]", split_result[1])
	fmt.Println("split_result[2]", split_result[2])
	fmt.Println("split_result[3]", split_result[3])

	if IsEVConnected, err := strconv.ParseInt(split_result[0], 10, 0); err != nil {
		log.Error("unable to convert IsEVConnected to int", err)
	} else {
		if IsEVConnected == 1 && evse.IsEVConnected == 0 {
			evse.IsEVConnected = 1
			evse.OnEVConnected_repeat()
			evse.OnEVConnected_fire_once()
			evse.OnEVConnected_fire_once = func() {}
		}
		if IsEVConnected == 0 && evse.IsEVConnected == 1 {
			evse.IsEVConnected = 0
			evse.OnEVDisconnected_repeat()
			evse.OnEVDisconnected_fire_once()
			evse.OnEVDisconnected_fire_once = func() {}
		}
	}

	if IsChargingEnabled, err := strconv.ParseInt(split_result[1], 10, 0); err != nil {
		log.Error("unable to convert IsChargingEnabled to int", err)
	} else {
		if IsChargingEnabled == 1 && evse.IsChargingEnabled == 0 {
			evse.IsChargingEnabled = 1
			evse.OnEVSEChargingEnabled_repeat()
			evse.OnEVSEChargingEnabled_fire_once()
			evse.OnEVSEChargingEnabled_fire_once = func() {}
		}
		if IsChargingEnabled == 0 && evse.IsChargingEnabled == 1 {
			evse.IsChargingEnabled = 0
			evse.OnEVSEChargingDisabled_repeat()
			evse.OnEVSEChargingDisabled_fire_once()
			evse.OnEVSEChargingDisabled_fire_once = func() {}
		}
	}

	if IsCharging, err := strconv.ParseInt(split_result[2], 10, 0); err != nil {
		log.Error("unable to convert IsCharging to int", err)
	} else {
		if IsCharging == 1 && evse.IsCharging == 0 {
			evse.IsCharging = 1
			evse.OnEVSEChargingStarted_repeat()
			evse.OnEVSEChargingStarted_fire_once()
			evse.OnEVSEChargingStarted_fire_once = func() {}

		}
		if IsCharging == 0 && evse.IsCharging == 1 {
			evse.IsCharging = 0
			evse.OnEVSEChargingStopped_repeat()
			evse.OnEVSEChargingStopped_fire_once()
			evse.OnEVSEChargingStopped_fire_once = func() {}
		}
	}

	if IsError, err := strconv.ParseInt(split_result[3], 10, 0); err != nil {
		log.Error("unable to convert IsError to int", err)
	} else {
		if IsError == 1 && evse.IsError == 0 {
			evse.IsError = 1
			evse.OnEVSEError_repeat()
			evse.OnEVSEError_fire_once()
			evse.OnEVSEError_fire_once = func() {}
		}
		if IsError == 0 && evse.IsError == 1 {
			evse.IsError = 0
			evse.OnEVSENoError_repeat()
			evse.OnEVSENoError_fire_once()
			evse.OnEVSENoError_fire_once = func() {}
		}
	}

}

func (evse *EVSE) updateMeterValues(meterValuesString string) {
	fmt.Println("Original meterValues string: ", meterValuesString)
	split_result := strings.Split(meterValuesString, ",")
	if len(split_result) != 2 {
		log.Error("Unable to update metervalues, meterValuesString is length is not 2")
		return
	}

	if EnergyActiveNet_kwh_times100, err := strconv.ParseInt(split_result[0], 10, 0); err != nil {
		log.Error("unable to convert EnergyActiveNet_kwh_times100 to int", err)
	} else {
		evse.EnergyActiveNet_kwh_times100 = EnergyActiveNet_kwh_times100
	}

	if PowerActiveImport_kw_times100, err := strconv.ParseInt(split_result[1], 10, 0); err != nil {
		log.Error("unable to convert PowerActiveImport_kw_times100 to int", err)
	} else {
		evse.PowerActiveImport_kw_times100 = PowerActiveImport_kw_times100
	}

}

func (evse *EVSE) statusPollLoop() {
	for {
		if len(messages_awaiting_resp) != 0 {
			log.Error("We are waiting for a EVSE reply, so not asking status yet")
			if time.Since(lastMessageSentAt) > time.Millisecond*5000 { // timeout
				_ = <-messages_awaiting_resp
			}
		} else {

			evse.send(AsyncEVSEMessage{
				Message: "status?\n",
				SuccessCallback: func(s string) {
					status_strings <- s

				},
			})
		}
		time.Sleep(time.Millisecond * 1000)
	}
}

func (evse *EVSE) metertValuesPollLoop() {
	for {
		if len(messages_awaiting_resp) != 0 {
			log.Error("We are waiting for a EVSE reply, so not asking status yet")
			if time.Since(lastMessageSentAt) > time.Millisecond*5000 { // timeout
				_ = <-messages_awaiting_resp
			}
		} else {
			evse.send(AsyncEVSEMessage{
				Message: "metervalues?\n",
				SuccessCallback: func(s string) {
					evse.updateMeterValues(s)
				},
			})
		}
		time.Sleep(time.Millisecond * 1000)
	}
}

func (evse *EVSE) send(message AsyncEVSEMessage) {
	out_channel <- message.Message
	messages_awaiting_resp <- message
}

func sendloop() {
	for out_message := range out_channel {
		fmt.Println("writing the following message to EVSE controller: ", out_message)
		_, err := tcp_conn.Write([]byte(out_message))
		if err != nil {
			println("Write to server failed:", err.Error())
			os.Exit(1)
		}

	}
}

func (evse *EVSE) StartCharge(onSuccess func(), onFailure func()) {
	if len(messages_awaiting_resp) != 0 {
		log.Error("We are waiting for a EVSE reply, so not asking status yet")
	} else {
		evse.send(AsyncEVSEMessage{
			Message: "start\n",
			SuccessCallback: func(reply string) {
				if reply == "OK" {
					onSuccess()
				} else {
					onFailure()
				}
			},
		})
		lastMessageSentAt = time.Now()
	}

}
