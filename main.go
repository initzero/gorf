package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"time"
	"fmt"
)

import (
	mux "github.com/gorilla/mux"
)

type Page struct {
	Title string
	Body  []byte
}

//this is bad but works

//if first 3 bytes are 020606 then its a page of servers
//reply with: 02060100(4 bytes from packet at offset 4)0a0000
//to parse:
//num servers in packet = (packetlength-15)/6 (i think num server count is actually in packet idk why i did it this way)
//then servers begin at offset 15 decimal
//4 bytes one for each ip octet then 16bit int of port, i think network byte order

//then at end youll get a packet starting with 020607 this is the end of list or whatever
//reply with 02060100(4 bytes from packet at offset 4)0a0000

// "173.242.119.102:18444"
// "75.19.98.179:7777"

const RF_DEDICATED byte = 0x01
const RF_NOTLAN byte = 0x02
const RF_PASSWORD byte = 0x04

// game versions
const RF_10 byte = 0x87
const RF_11 byte = 0x87
const RF_12 byte = 0x89
const RF_13 byte = 0x91

// game types
const RF_DM byte = 0x00
const RF_CTF byte = 0x01
const RF_TEAMDM byte = 0x02

// join
const RF_JOIN uint16 = 0x02
const RF_JOIN_FAILED uint16 = 0x04
const RF_JOIN_SUCCESS uint16 = 0x03

// join fail reason
const RF_WRONG_PASS byte = 0x03
const RF_BANNED byte = 0x0a
const RF_MAP_CHANGE byte = 0x05

// Join
type JoinServer struct {
	Type       uint16 // 0x200 RFJoin
	Size       uint16
	Version    uint8
	Name       []byte
	Undefined  uint32   //0x05
	Password   []byte   // use "" if no password
	ConnSpeed  uint32   // speed in bytes/s (56k - 0x0a28)
	Undefined2 [16]byte // 7e 40 c2 1a 00 c0 13 00 00 00 00 00 00 00 00 00
	//Undefined2 [32]byte //only v1.0 (22 3b 68 3a 00 20 f0 00 ee 82 b2 7e 00 e0 f6 00 cd 0d 66 92 00 c0 13 00 00 00 00 00 00 00 00 00)
}

type JoinServerFailed struct {
	Type   uint16 // 0x400 RFJoinFailed
	Size   uint16 // size of rest of pkg
	Reason uint8
}

type JoinServerSuccess struct {
	Type       uint16 //0x300
	Size       uint16
	Level      string
	Undefined  uint32
	GameType   uint8   // RFDM, RFCTF, RFTeamDM
	Undefined2 [7]byte // 00 00 00 12 24 00 00
	LevelTime  float32
	TimeLimit  float32
	Id         uint8 // our ID
}

// this describes info from the first
// tracker query, which replies with
// list of IPs and Ports
type ServerIP struct {
	Addr string
	Port int32
	Info ServerInfo
}

// Send this packet to get initial list of IP addrs and Ports
var serverListPkt []byte = []byte{
	0x02, 0x06, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, 0x00, 0x00,
}

// ack
var ackPkt []byte = []byte{
	0x02, 0x06, 0xFF, 0x01, 0x00, 0x0a,
}

// Server info request packet
var serverInfoPkt []byte = []byte{
	0x00, 0x00, 0x00, 0x00,
}

// tracker sends this after the server info:
// 02 06 07 00 09 cf 16 00 0a 00

func checkErr(err error, context string) {
	if err != nil {
		log.Printf("%s", context)
		log.Printf("%s", err)
	}
}

// Function to serialize generic struct to byte buffer
func writeStructFields(s interface{}, buf *bytes.Buffer) error {
	r := reflect.ValueOf(s).Elem()
	var err error
	for i := 0; i < r.NumField(); i++ {
		f := r.Field(i)
		err = binary.Write(buf, binary.BigEndian, f.Interface())
		checkErr(err, "write bytes")
	}
	return err
}

// Join
// type JoinServer struct {
// 	Type       uint16 // 0x200 RFJoin
// 	Size       uint16
// 	Version    uint8
// 	Name       []byte
// 	Undefined  uint32   //0x05
// 	Password   []byte   // use "" if no password
// 	ConnSpeed  uint32   // speed in bytes/s (56k - 0x0a28)
// 	Undefined2 [16]byte // 7e 40 c2 1a 00 c0 13 00 00 00 00 00 00 00 00 00
// 	//Undefined2 [32]byte //only v1.0 (22 3b 68 3a 00 20 f0 00 ee 82 b2 7e 00 e0 f6 00 cd 0d 66 92 00 c0 13 00 00 00 00 00 00 00 00 00)
// }

// Valid:
// 00 02 28 00 89 55 63 68 69 6d 61 20 53 61 73 6b 69 65 00 05 00 00 00 00 28 0a 00 00 7e 40 c2 1a 00 c0 13 00 00 00 00 00 00 00 00 00 

// Current:
// 02 00 28 00 91 55 63 68 69 6d 61 20 53 61 73 6b 69 65 05 00 00 00 28 0a 00 00 7e 40 c2 1a 00 c0 13 00 00 00 00 00 00 00 00 00

// Current (BigEndian):
// 00 02 00 28 91 55 63 68 69 6d 61 20 53 61 73 6b 69 65 00 00 00 05 00 00 0a 28 7e 40 c2 1a 00 c0 13 00 00 00 00 00 00 00 00 00 

func joinServer(ip string, port int32) {
	localAddr, err := net.ResolveUDPAddr("udp", ":0")
	checkErr(err, "resolve local")

	remoteAddr, err := net.ResolveUDPAddr("udp",
		ip+":"+strconv.Itoa(int(port)))
	checkErr(err, "resolve server")

	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	checkErr(err, "dial server")
	//conn.SetDeadline(time.Now().Add(3e9))
	joinPayload := JoinServer{}
	joinPayload.Type = RF_JOIN
	joinPayload.Version = RF_12
	joinPayload.Name = []byte("[NSA] Prism")
	joinPayload.Name = append(joinPayload.Name, 0x00)
	joinPayload.Undefined = 0x050 << 24							// some junk
	joinPayload.Password = []byte("cold")
	joinPayload.Password = append(joinPayload.Password, 0x00)
	joinPayload.ConnSpeed = (0x28 << 8) | 0x0a 					// l0l
	joinPayload.Undefined2 = [16]byte{
		0x7e, 0x40, 0xc2, 0x1a, 0x00, 0xc0, 0x13, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	// add up the struct field sizes - type
	joinPayload.Size = (uint16(25 + len(joinPayload.Name) + len(joinPayload.Password)) << 8)

	buf := new(bytes.Buffer)
	err = writeStructFields(&joinPayload, buf)
	checkErr(err, "before write")

	var resp []byte = make([]byte, 1024)
	var gotPkt chan int = make(chan int, 1)
	go func(conn *net.UDPConn, buffer []byte, gotPkt chan int) {
		counter := 0
		for {
			ret, err := conn.Read(buffer)
			checkErr(err, "info read goroutine")
			gotPkt <- ret
			counter += 1
			if counter > 1 {
				close(gotPkt)
				break
			}
		}
	}(conn, resp, gotPkt)

	conn.Write(buf.Bytes())
	
	for bytesRead := range gotPkt {
		fmt.Printf("%#v\n", resp[0:bytesRead])
	}

	haveMapPayload := []byte{
		0x01, 0x03, 0x00, 0x42, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	conn.Write(haveMapPayload)
	// 01:03:00:42:01:00:00:00:00:00:00
	gotPkt = make(chan int, 1)
	go func(conn *net.UDPConn, buffer []byte, gotPkt chan int) {
		counter := 0
		for {
			ret, err := conn.Read(buffer)
			checkErr(err, "info read goroutine")
			gotPkt <- ret
			counter += 1
			if counter > 0 {
				close(gotPkt)
				break
			}
		}
	}(conn, resp, gotPkt)

	for bytesRead := range gotPkt {
		fmt.Printf("%#v\n", resp[0:bytesRead])
	}
	unknownPayload := []byte {
		0x01, 0x06, 0x00, 0xbd, 0xfe, 0x00, 0x28, 0x00, 0x50, 0x2f,	
	}
	conn.Write(unknownPayload)

	gotPkt = make(chan int, 1)
	go func(conn *net.UDPConn, buffer []byte, gotPkt chan int) {
		for {
			ret, err := conn.Read(buffer)
			fmt.Printf("read: %#v\n", buffer[0:ret])
			checkErr(err, "info read goroutine")
			if ret == 4 {
				if buffer[1] == 0x18 {
					conn.Write([]byte{0x00, 0x19, 0x00, 0x00})
				}
			}
			
		}
	}(conn, resp, gotPkt)
}

func parseTrackerResponse(resp []byte, pipe chan []ServerIP) {
	// each server entry is 6 bytes
	// first 4 bytes are IP(a, b, c, d)
	// next 2 bytes are Port(a << 4 | b)
	numServers := (len(resp) - 15) / 6
	log.Println("numServers: ", numServers)
	log.Printf("tracker resp: %#v", resp)
	data := resp[15:]
	srvSlice := []ServerIP{}
	srvInfo := ServerIP{Addr: "", Port: 0}
	for i := 0; i < numServers; i++ {
		start := data[0+(6*i) : 6+(6*i)]
		srvInfo.Addr = net.IPv4(start[0], start[1], start[2], start[3]).String()
		srvInfo.Port = int32(start[4])<<8 | int32(start[5])
		srvSlice = append(srvSlice, srvInfo)
	}
	pipe <- srvSlice
}

// Response from Server info query
type ServerInfo struct {
	Type       uint16
	Size       uint16
	Version    uint8
	Name       []byte
	GameType   uint8
	Players    uint8
	MaxPlayers uint8
	Level      []byte
	Mod        []byte
	Flags      uint8
}

func parseServerInfoResponse(resp []byte) ServerInfo {
	// pos tracks where we left off for variable
	// length fields like server name
	var pos int = 0

	var s ServerInfo
	s.Type = uint16(resp[0]<<8) | uint16(resp[1])
	s.Size = uint16(resp[2]<<8) | uint16(resp[3])
	s.Version = uint8(resp[4])

	//search name until 0x00
	name := []byte{}
	for i, v := range resp[5:] {
		if v == 0x00 {
			pos = 5 + i + 1
			break
		} else {
			name = append(name, v)
		}
	}
	s.Name = name
	s.GameType = uint8(resp[pos])
	s.Players = uint8(resp[pos+1])
	s.MaxPlayers = uint8(resp[pos+2])

	//get level name
	level := []byte{}
	//rebase the pos
	pos = pos + 3
	for i, v := range resp[pos:] {
		if v == 0x00 {
			pos = pos + i + 1
			break
		} else {
			level = append(level, v)
		}
	}
	s.Level = level

	//get mod name
	mod := []byte{}
	for i, v := range resp[pos:] {
		if v == 0x00 {
			pos = pos + i + 1
			break
		} else {
			mod = append(mod, v)
		}
	}
	s.Mod = mod
	s.Flags = uint8(resp[pos])
	log.Printf("%#v", s)
	return s
}

func RFServerInfo(server ServerIP, pipe chan ServerIP) {
	gotPkt := make(chan int, 1)
	var resp []byte = make([]byte, 128)

	localAddr, err := net.ResolveUDPAddr("udp", ":0")
	checkErr(err, "resolve local")

	remoteAddr, err := net.ResolveUDPAddr("udp",
		server.Addr+":"+strconv.Itoa(int(server.Port)))
	checkErr(err, "resolve server")

	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	conn.SetDeadline(time.Now().Add(3e9))

	//listen and get our pkt
	go func(conn *net.UDPConn, buffer []byte, gotPkt chan int) {
		ret, err := conn.Read(buffer)
		checkErr(err, "info read goroutine")
		gotPkt <- ret
	}(conn, resp, gotPkt)

	// send request
	_, err = conn.Write(serverInfoPkt)
	checkErr(err, "Write to server")

	bRead := <-gotPkt
	conn.Close()
	if bRead > 0 {
		server.Info = parseServerInfoResponse(resp[0:bRead])
	}
	pipe <- server
	go joinServer(server.Addr, server.Port)
}

// get the RF server IPs and Ports
// publish them with json response
func RFServers(w http.ResponseWriter, r *http.Request) {
	pipe := make(chan []ServerIP, 1)
	sInfoPipe := make(chan ServerIP, 1)
	var resp []byte = make([]byte, 1024)

	// setup our addrs
	localAddr, err := net.ResolveUDPAddr("udp", ":9000")
	checkErr(err, "Resolve localAddr")
	remoteAddr, err := net.ResolveUDPAddr("udp", "173.242.119.102:18444")
	checkErr(err, "Resolve remoteAddr")

	// listen for stuff
	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	checkErr(err, "DialUDP")

	// send serverListPkt to tracker
	ret, err := conn.Write(serverListPkt)
	checkErr(err, "Write tracker")
	log.Println(ret)

	ret, err = conn.Read(resp)
	checkErr(err, "read tracker")

	conn.Close()

	go parseTrackerResponse(resp[0:ret], pipe)

	// wait for pipe signal
	servers := []ServerIP{}
	servers = <-pipe

	counter := 0
	finished := make(chan bool, 1)

	for _, srv := range servers {
		go RFServerInfo(srv, sInfoPipe)
		counter += 1
		log.Println(counter)
	}

	servers = []ServerIP{}

	// lol srsly
harvester:
	for {
		select {
		case s := <-sInfoPipe:
			servers = append(servers, s)
			counter -= 1
			if counter == 0 {
				finished <- true
			}
		case <-finished:
			break harvester
			// case <-time.After(3e9):
			// 	break harvester
		}
	}
	log.Printf("%#v", servers)
	encoder := json.NewEncoder(w)
	err = encoder.Encode(servers)
	checkErr(err, "json encode")
}

// render homepage
func Home(w http.ResponseWriter, r *http.Request) {
	title := "rf"
	p := &Page{Title: title, Body: []byte("rf")}
	t, err := template.ParseFiles("templates/home.html")
	if err != nil {
		log.Fatal(err)
	}

	// cache this html; if deploying something new, users will need to shift+f5 their browsers
	// to drop their local cache
	w.Header().Set("Cache-control", "private, max-age=5184000")

	t.Execute(w, p)
}

func main() {
//	go joinServer("192.95.22.53", 7759)
	r := mux.NewRouter()
	r.HandleFunc("/", Home)
	r.HandleFunc("/rf/servers", RFServers)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.ListenAndServe(":8080", r)
}
