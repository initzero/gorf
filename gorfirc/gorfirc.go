package gorfirc

import (
	"log"
	"strconv"
	"strings"
)

import (
	irc "github.com/thoj/go-ircevent"
	"rflib"
)

func checkErr(err error, context string) {
	if err != nil {
		log.Printf("%s", context)
		log.Printf("%s", err)
	}
}

// send strings to IRC
func sendIRC(s []string, i *irc.Connection, e *irc.Event, ready chan bool) {
	i.SendRaw("PRIVMSG " + e.Arguments[0] + " :" + e.Message)
	ready <- true
}

// type ServerInfo struct {
// 	Type       uint16
// 	Size       uint16
// 	Version    uint8
// 	Name       []byte
// 	GameType   uint8
// 	Players    uint8
// 	MaxPlayers uint8
// 	Level      []byte
// 	Mod        []byte
// 	Flags      uint8
// }

func buildServerString(s rflib.ServerIP) string {
	info := s.Info
	
	var version string
	switch(info.Version) {

	case rflib.RF_11:
		version = "1.1"
	case rflib.RF_12:
		version = "1.2"
	case rflib.RF_13:
		version = "1.3"
	default:
		strconv.Itoa(int(info.Version))
	}

	name := string(info.Name[:])
	
	var gameType string
	switch(info.GameType) {
	case rflib.RF_DM:
		gameType = "Deathmatch"
	case rflib.RF_TEAMDM:
		gameType = "Team Deathmatch"
	case rflib.RF_CTF:
		gameType = "CTF"
	default:
		gameType = strconv.Itoa(int(info.GameType))
	}
	
	players := strconv.Itoa(int(info.Players)) + "/" + strconv.Itoa(int(info.MaxPlayers))
	
	level := string(info.Level[:])
	
	mod := string(info.Mod[:])
	
	return name + " " + s.Addr + ":" + strconv.Itoa(int(s.Port)) + " " + version + " " + gameType + " " + players + " " + level + " " + mod
}

func sendServerListToIRC(servers []rflib.ServerIP, i *irc.Connection, e *irc.Event, showEmpty bool, ready chan bool) {
	for _, v := range servers {
		if v.Info.Players > 0 || showEmpty == true {
			i.SendRaw("PRIVMSG " + e.Arguments[0] + " : " + buildServerString(v))
		}
	}
	ready <- true
}

// server [addr:port]
func SetupIRC(server string, channel string) {
	icon := irc.IRC("gorfbot", "sacredrear")
	err := icon.Connect(server)
	checkErr(err, "irc connect")
	rdy := make(chan bool, 1)
	// join a channel
	icon.AddCallback("001", func(e *irc.Event) { icon.Join(channel) })

	icon.AddCallback("PRIVMSG", func(e *irc.Event) {
			if strings.HasPrefix(e.Message, "@rf") {
				if len(e.Message) < 4 {
					return
				}
				split := strings.Split(e.Message, " ")
				if split[1] == "server-list" {
					servers := rflib.RFServers()
					if len(split) > 2 {
						if split[2] == "empty" {
							sendServerListToIRC(servers, icon, e, true, rdy)
						} else if split[2] == "no-empty" {
							sendServerListToIRC(servers, icon, e, false, rdy)
						} else {
							icon.SendRaw("PRIVMSG " + e.Arguments[0] + " : invalid option: " + split[2])
						}
					}
					<-rdy
				} else if split[1] == "join" {
					if len(split) == 4 {
						go rflib.JoinServerByIP(split[2] + ":" + split[3])
					}
				}
			}
		})
}