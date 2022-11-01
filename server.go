// Copyright 2015 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// CREATED FROM https://github.com/gorilla/websocket/tree/master/examples/echo
// CREATED FROM https://github.com/pion/webrtc/tree/master/examples/save-to-disk
// REQUIRES MODULE MODE
//   export GO111MODULE=on

//go:build ignore
// +build ignore

package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	//"os"

	"github.com/gorilla/websocket"
	//"intercom/main/save-to-disk"
	
	//Save to disk
	"time"
	"fmt"
	"strings"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/mike-drucker/intercom/internal/signal"
	//"github.com/pion/webrtc/v3/examples/internal"
	"github.com/pion/webrtc/v3/pkg/media"
	//"github.com/pion/webrtc/v3/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)


func saveToDisk(i media.Writer, track *webrtc.TrackRemote) {
	defer func() {
		if err := i.Close(); err != nil {
			panic(err)
		}
	}()

	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			panic(err)
		}
		if err := i.WriteRTP(rtpPacket); err != nil {
			panic(err)
		}
	}
}

var addr = flag.String("addr", ":9001", "http service address")

var upgrader = websocket.Upgrader{} // use default options

func echo(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		if len(message) > 100 {
		  receive(string(message),c)
		  continue
		}
		
		log.Printf("recv: %s", message)
		err = c.WriteMessage(mt, message)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func home(w http.ResponseWriter, r *http.Request) {
	homeTemplate.Execute(w, "wss://"+r.Host+"/intercom/echo")
}

func receive(offerString string, c *websocket.Conn) {
   //webrtc save-to-disk section
  // Create a MediaEngine object to configure the supported codec
	m := &webrtc.MediaEngine{}

	// Setup the codecs you want to use.
	// We'll use a VP8 and Opus but you can also define your own
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		panic(err)
	}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}

	// Create a InterceptorRegistry. This is the user configurable RTP/RTCP Pipeline.
	// This provides NACKs, RTCP Reports and other features. If you use `webrtc.NewPeerConnection`
	// this is enabled by default. If you are manually managing You MUST create a InterceptorRegistry
	// for each PeerConnection.
	i := &interceptor.Registry{}

	// Use the default set of Interceptors
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		panic(err)
	}
	// Create the API object with the MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}
	// Allow us to receive 1 audio track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}

	oggFile, err := oggwriter.New("output.ogg", 48000, 2)
	if err != nil {
		panic(err)
	}
	
	// Set a handler for when a new remote track starts, this handler saves buffers to disk as
	// an ivf file, since we could have multiple video tracks we provide a counter.
	// In your application this is where you would handle/process video
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				errSend := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
				if errSend != nil {
					fmt.Println(errSend)
				}
			}
		}()

		codec := track.Codec()
		if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
			fmt.Println("Got Opus track, saving to disk as output.opus (48 kHz, 2 channels)")
			saveToDisk(oggFile, track)
		}
	})
	
	// parse offer string
	offer := webrtc.SessionDescription{}
	signal.Decode(offerString, &offer)
	
  // Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}
	
	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}
	
	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	
	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}
	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete
	//send
	c.WriteMessage(0,[]byte(signal.Encode(*peerConnection.LocalDescription())))
	
	// Block forever
	select {}
	
}


func main() {
  
  //websocket-echo section
	flag.Parse()
	log.SetFlags(0)
	http.HandleFunc("/echo", echo)
	http.HandleFunc("/", home)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

var homeTemplate = template.Must(template.New("").Parse(`
<html>
  <head>
    <script>

    /*WS SECTION*/
    window.addEventListener("load", function(evt) {
     var output = document.getElementById("output");
      var input = document.getElementById("input");
      var ws;
      var print = function(message) {
          var d = document.createElement("div");
          d.textContent = message;
          output.appendChild(d);
          output.scroll(0, output.scrollHeight);
      };
      document.getElementById("open").onclick = function(evt) {
          if (ws) {
              return false;
          }
          ws = new WebSocket("{{.}}");
          window.ws = ws;
          
          ws.onopen = function(evt) {
              print("OPEN");
          }
          ws.onclose = function(evt) {
              print("CLOSE");
              ws = null;
          }
          ws.onmessage = function(evt) {
              print("RESPONSE: " + evt.data);
          }
          ws.onerror = function(evt) {
              print("ERROR: " + evt.data);
          }
          return false;
      };
      document.getElementById("send").onclick = function(evt) {
          if (!ws) {
              return false;
          }
          print("SEND: " + input.value);
          ws.send(input.value);
          return false;
      };
      document.getElementById("close").onclick = function(evt) {
          if (!ws) {
              return false;
          }
          ws.close();
          return false;
      };
    });
    
/* WEBRTC SECTION */
      var log = (msg,e=null) => {
        document.getElementById('logs').innerHTML += msg + '<br>'
        console.log(msg,e);
      }
      var error = (msg,e=null) => {
        document.getElementById('logs').innerHTML += msg + '<br>'
        console.error(msg,e);
      }
      
      const pc = new RTCPeerConnection({iceServers: [{urls: 'stun:stun.l.google.com:19302'}]}); //pc.close();
      
      pc.oniceconnectionstatechange = (e) => log;
      /*
      pc.createOffer()
        .then((offer) => pc.setLocalDescription(offer))
        .then((x)=> {
            console.log(pc.localDescription);
            console.log(JSON.stringify(pc.localDescription));
            console.log(btoa(JSON.stringify(pc.localDescription)));
      });*/
     
      navigator.mediaDevices.getUserMedia({audio: true}).then((stream) => {
        // Create an AudioNode from the stream.
        console.log('AAAAAAAAAAAA');
        stream.getTracks().forEach(track => pc.addTrack(track, stream))
        pc.stream = stream;
        pc.createOffer().then(
          d => {
            pc.setLocalDescription(d).then((x)=> {
              log(pc.localDescription);
              log(btoa(JSON.stringify(pc.localDescription)));
              document.getElementById('localSessionDescription').innerText = btoa(JSON.stringify(pc.localDescription));
            });
        }).catch(error);
      }).catch(error);
      
      window.startSession = () => {
          let sd = document.getElementById('remoteSessionDescription').value
          if (sd === '') {
            return alert('Session Description must not be empty');
          }
        
          try {
            pc.setRemoteDescription(JSON.parse(atob(sd)));
          } catch (e) {
            alert(e);
          }
      }
        
    //ffmpeg -i /config/www/output.ogg -f alsa default
      
    </script>
  </head>
  <body>
      Browser base64 Session Description<br />
<textarea id="localSessionDescription" readonly="true"></textarea> <br />
<button onclick="window.copySDP()">
        Copy browser SDP to clipboard
</button>
<br />
<br />

Golang base64 Session Description<br />
<textarea id="remoteSessionDescription"></textarea> <br/>
<button onclick="window.startSession()"> Start Session </button><br />
<button onclick="pc.close()"> Close Session </button><br />
<br />

Logs<br />
<div id="logs"></div>
<table>
<tr><td valign="top" width="50%">
<p>Click "Open" to create a connection to the server, 
"Send" to send a message to the server and "Close" to close the connection. 
You can change the message and send multiple times.
<p>
<form>
<button id="open">Open</button>
<button id="close">Close</button>
<p><input id="input" type="text" value="Hello world!">
<button id="send">Send</button>
</form>
</td><td valign="top" width="50%">
<div id="output" style="max-height: 70vh;overflow-y: scroll;"></div>
</td></tr></table>
  </body>
</html>
`))
