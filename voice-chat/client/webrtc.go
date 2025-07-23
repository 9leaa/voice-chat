package main

import (
	"github.com/pion/mediadevices"
	"github.com/pion/webrtc/v3"
)

func init() {
	mediadevices.SetCodecSelector(mediadevices.DefaultCodecSelector)
}

type WebRTCClient struct {
	PeerConnection *webrtc.PeerConnection
}

func NewWebRTCClient() (*WebRTCClient, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	mediaEngine.RegisterCodec(webrtc.NewRTPOpusCodec(webrtc.DefaultPayloadTypeOpus, 48000))
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
	)
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	return &WebRTCClient{
		PeerConnection: peerConnection,
	}, nil
}
