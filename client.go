package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/maxmcd/webtty/pkg/sd"
	"github.com/pion/webrtc/v3"
	"golang.org/x/crypto/ssh/terminal"
)

type clientSession struct {
	session
	dc          *webrtc.DataChannel
	offerString string
}

func (cs *clientSession) dataChannelOnMessage() func(payload webrtc.DataChannelMessage) {
	return func(p webrtc.DataChannelMessage) {
		if p.IsString {
			if string(p.Data) == "quit" {
				if cs.isTerminal {
					terminal.Restore(int(os.Stdin.Fd()), cs.oldTerminalState)
				}
				cs.errChan <- nil
				return
			}
			cs.errChan <- fmt.Errorf(`Unmatched string message: "%s"`, string(p.Data))
		} else {
			f := bufio.NewWriter(os.Stdout)
			f.Write(p.Data)
			f.Flush()
		}
	}
}

func (cs *clientSession) run() (err error) {
	if err = cs.init(); err != nil {
		return
	}

	maxPacketLifeTime := uint16(1000) // Arbitrary
	ordered := true
	if cs.dc, err = cs.pc.CreateDataChannel("data", &webrtc.DataChannelInit{
		Ordered:           &ordered,
		MaxPacketLifeTime: &maxPacketLifeTime,
	}); err != nil {
		log.Println(err)
		return
	}

	cs.dc.OnOpen(cs.dataChannelOnOpen())
	cs.dc.OnMessage(cs.dataChannelOnMessage())

	if cs.offer, err = sd.Decode(cs.offerString); err != nil {
		log.Println(err)
		return
	}
	if cs.offer.Key != "" {
		if err = cs.offer.Decrypt(); err != nil {
			log.Println(err)
			return
		}
	}
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  cs.offer.Sdp,
	}

	if err = cs.pc.SetRemoteDescription(offer); err != nil {
		log.Println(err)
		return err
	}
	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := cs.pc.CreateAnswer(nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(cs.pc)

	err = cs.pc.SetLocalDescription(answer)
	if err != nil {
		log.Println(err)
		return
	}

	// Block until ICE Gathering is complete
	<-gatherComplete

	answerSd := sd.SessionDescription{
		Sdp:   cs.pc.LocalDescription().SDP,
		Key:   cs.offer.Key,
		Nonce: cs.offer.Nonce,
	}
	if cs.offer.Key != "" {
		// Encrypt with the shared keys from the offer
		_ = answerSd.Encrypt()

		// Don't upload the keys, the host has them
		answerSd.Key = ""
		answerSd.Nonce = ""
	}

	encodedAnswer := sd.Encode(answerSd)
	if cs.offer.TenKbSiteLoc == "" {
		fmt.Printf("Answer created. Send the following answer to the host:\n\n")
		fmt.Println(encodedAnswer)
	} else {
		if err := create10kbFile(cs.offer.TenKbSiteLoc, encodedAnswer); err != nil {
			return err
		}
	}
	err = <-cs.errChan
	cs.cleanup()
	return err
}
