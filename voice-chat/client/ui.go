package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
	hook "github.com/robotn/gohook"
)

type UIClient struct {
	App              fyne.App
	Window           fyne.Window
	CurrentUser      string
	ContactList      *widget.List
	Contacts         []string
	TalkButton       *widget.Button
	StatusLabel      *widget.Label
	WSConnection     *WebSocketClient
	VoiceActive      bool
	HotkeyEntry      *widget.Entry
	VoiceMode        string
	FreeTalkToggle   *widget.Button
	stopHotkeyListen chan struct{}
	progressDialog   dialog.Dialog
	WebRTC           map[string]*WebRTCClient
	MediaStream      mediadevices.MediaStream
}

func NewUIClient() *UIClient {
	app := app.New()
	window := app.NewWindow("轻量语音聊天")
	window.Resize(fyne.NewSize(400, 550))
	return &UIClient{
		App:              app,
		Window:           window,
		Contacts:         []string{},
		HotkeyEntry:      widget.NewEntry(),
		VoiceMode:        "pushToTalk",
		stopHotkeyListen: make(chan struct{}),
		WebRTC:           make(map[string]*WebRTCClient),
	}
}
func (ui *UIClient) Start() {
	ui.setupLoginUI()
	ui.Window.Show()
	ui.App.Run()
}
func (ui *UIClient) setupLoginUI() {
	username := widget.NewEntry()
	password := widget.NewPasswordEntry()
	serverAddr := widget.NewEntry()
	serverAddr.SetText("localhost:8080")
	form := widget.NewForm(
		widget.NewFormItem("用户名", username),
		widget.NewFormItem("密码", password),
		widget.NewFormItem("服务器地址", serverAddr),
	)
	form.OnSubmit = func() {
		if username.Text == "" {
			dialog.ShowError(fmt.Errorf("用户名不能为空"), ui.Window)
			return
		}
		ui.CurrentUser = username.Text
		ui.setupMainUI(serverAddr.Text)
	}
	ui.Window.SetContent(container.NewVBox(
		widget.NewLabel("轻量语音聊天 v1.0"),
		widget.NewLabel("登录使用"),
		form,
	))
}
func (ui *UIClient) setupMainUI(serverAddr string) {
	progressLabel := widget.NewLabel("正在连接到服务器...")
	ui.progressDialog = dialog.NewCustom("连接中", "取消", progressLabel, ui.Window)
	ui.progressDialog.Show()
	go func() {
		wsClient := NewWebSocketClient(serverAddr, ui.CurrentUser)
		if wsClient == nil {
			dialog.ShowError(fmt.Errorf("连接服务器失败"), ui.Window)
			return
		}
		ui.WSConnection = wsClient
		ui.progressDialog.Hide()
		ui.WSConnection.ReadMessages(func(msgType string, data json.RawMessage) {
			switch msgType {
			case "userlist":
				var users []string
				if err := json.Unmarshal(data, &users); err != nil {
					log.Printf("解析用户列表错误: %v", err)
					return
				}
				ui.UpdateUserList(users)
			case "offer", "answer", "candidate":
				var signal struct {
					From string          `json:"from"`
					Data json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(data, &signal); err != nil {
					log.Printf("解析信令错误: %v", err)
					return
				}
				ui.handleWebRTCSignal(msgType, signal.From, signal.Data)
			}
		})
		modeGroup := widget.NewRadioGroup([]string{"按键说话 (PTT)", "自由说话 (始终开启)"}, func(selected string) {
			if selected == "按键说话 (PTT)" {
				ui.VoiceMode = "pushToTalk"
				ui.FreeTalkToggle.Hide()
				ui.TalkButton.Show()
			} else {
				ui.VoiceMode = "freeTalk"
				ui.TalkButton.Hide()
				ui.FreeTalkToggle.Show()
			}
		})
		modeGroup.SetSelected("按键说话 (PTT)")
		ui.HotkeyEntry.SetText("v")
		hotkeyForm := widget.NewForm(
			widget.NewFormItem("通话热键", ui.HotkeyEntry),
		)
		ui.ContactList = widget.NewList(
			func() int { return len(ui.Contacts) },
			func() fyne.CanvasObject { return widget.NewLabel("") },
			func(i widget.ListItemID, o fyne.CanvasObject) {
				o.(*widget.Label).SetText(ui.Contacts[i])
			},
		)
		ui.TalkButton = widget.NewButton("按住说话 (V键)", func() {})
		ui.TalkButton.Importance = widget.HighImportance
		ui.FreeTalkToggle = widget.NewButtonWithIcon("开始自由说话", theme.MediaRecordIcon(), func() {
			if ui.VoiceActive {
				ui.VoiceActive = false
				ui.FreeTalkToggle.SetIcon(theme.MediaRecordIcon())
				ui.FreeTalkToggle.SetText("开始自由说话")
				if ui.MediaStream != nil {
					ui.MediaStream.Close()
					ui.MediaStream = nil
				}
			} else {
				ui.VoiceActive = true
				go ui.startVoiceCapture()
				ui.FreeTalkToggle.SetIcon(theme.MediaStopIcon())
				ui.FreeTalkToggle.SetText("停止自由说话")
			}
		})
		ui.FreeTalkToggle.Importance = widget.HighImportance
		ui.FreeTalkToggle.Hide()
		saveHotkeyBtn := widget.NewButton("保存热键", func() {
			ui.TalkButton.SetText("按住说话 (" + ui.HotkeyEntry.Text + "键)")
		})
		ui.StatusLabel = widget.NewLabel("连接成功")
		mainContent := container.NewBorder(
			container.NewVBox(
				widget.NewLabel("用户: "+ui.CurrentUser),
				widget.NewLabel("说话模式:"),
				modeGroup,
				hotkeyForm,
				container.NewHBox(saveHotkeyBtn),
				widget.NewLabel("联系人列表"),
			),
			container.NewVBox(
				ui.StatusLabel,
				container.NewHBox(layout.NewSpacer(), ui.TalkButton, layout.NewSpacer()),
				container.NewHBox(layout.NewSpacer(), ui.FreeTalkToggle, layout.NewSpacer()),
			),
			nil, nil,
			ui.ContactList,
		)
		ui.Window.SetContent(mainContent)
		ui.listenGlobalHotkey()
	}()
}
func (ui *UIClient) handleWebRTCSignal(signalType, from string, data json.RawMessage) {
	rtc, exists := ui.WebRTC[from]
	if !exists {
		var err error
		rtc, err = NewWebRTCClient()
		if err != nil {
			log.Printf("创建WebRTC连接失败: %v", err)
			return
		}
		ui.WebRTC[from] = rtc

		rtc.PeerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate != nil {
				candidateJSON := candidate.ToJSON()
				ui.WSConnection.SendSignal("candidate", from, candidateJSON)
			}
		})

		rtc.PeerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			log.Printf("收到来自 %s 的音频轨道", from)
			for {
				_, _, err := track.ReadRTP()
				if err != nil {
					log.Printf("读取RTP包失败: %v", err)
					return
				}
			}
		})
	}
	switch signalType {
	case "offer":
		var sdp string
		if err := json.Unmarshal(data, &sdp); err != nil {
			log.Printf("解析offer失败: %v", err)
			return
		}

		offer := webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  sdp,
		}

		if err := rtc.PeerConnection.SetRemoteDescription(offer); err != nil {
			log.Printf("设置远程描述失败: %v", err)
			return
		}

		answer, err := rtc.PeerConnection.CreateAnswer(nil)
		if err != nil {
			log.Printf("创建应答失败: %v", err)
			return
		}

		if err = rtc.PeerConnection.SetLocalDescription(answer); err != nil {
			log.Printf("设置本地描述失败: %v", err)
			return
		}

		ui.WSConnection.SendSignal("answer", from, answer.SDP)

	case "answer":
		var sdp string
		if err := json.Unmarshal(data, &sdp); err != nil {
			log.Printf("解析answer失败: %v", err)
			return
		}

		answer := webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  sdp,
		}

		if err := rtc.PeerConnection.SetRemoteDescription(answer); err != nil {
			log.Printf("设置远程描述失败: %v", err)
		}

	case "candidate":
		var candidate webrtc.ICECandidateInit
		if err := json.Unmarshal(data, &candidate); err != nil {
			log.Printf("解析candidate失败: %v", err)
			return
		}

		if err := rtc.PeerConnection.AddICECandidate(candidate); err != nil {
			log.Printf("添加ICE候选失败: %v", err)
		}
	}
}
func (ui *UIClient) listenGlobalHotkey() {
	go func() {
		evChan := hook.Start()
		defer hook.End()
		for {
			select {
			case <-ui.stopHotkeyListen:
				return
			case ev := <-evChan:
				if ev.Kind == hook.KeyDown || ev.Kind == hook.KeyUp {
					hotkey := strings.ToLower(ui.HotkeyEntry.Text)
					if hotkey == "" {
						hotkey = "v"
					}
					key := ""
					if ev.Keychar != 0 {
						key = strings.ToLower(string(ev.Keychar))
					} else if ev.Rawcode != 0 {
						switch ev.Rawcode {
						case 162:
							key = "ctrl"
						case 164:
							key = "alt"
						case 160:
							key = "shift"
						case 32:
							key = "space"
						case 13:
							key = "enter"
						}
					}
					if key == "" {
						continue
					}
					if ev.Kind == hook.KeyDown && key == hotkey {
						if ui.VoiceMode == "pushToTalk" && !ui.VoiceActive {
							ui.VoiceActive = true
							go ui.startVoiceCapture()
						}
					}
					if ev.Kind == hook.KeyUp && key == hotkey {
						if ui.VoiceMode == "pushToTalk" && ui.VoiceActive {
							ui.VoiceActive = false
							if ui.MediaStream != nil {
								ui.MediaStream.Close()
								ui.MediaStream = nil
							}
						}
					}
				}
			}
		}
	}()
}
func (ui *UIClient) UpdateUserList(users []string) {
	var filtered []string
	for _, user := range users {
		if user != ui.CurrentUser {
			filtered = append(filtered, user)

			if _, exists := ui.WebRTC[user]; !exists {
				rtc, err := NewWebRTCClient()
				if err != nil {
					log.Printf("创建WebRTC连接失败: %v", err)
					continue
				}
				ui.WebRTC[user] = rtc

				rtc.PeerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
					if candidate != nil {
						ui.WSConnection.SendSignal("candidate", user, candidate.ToJSON())
					}
				})

				rtc.PeerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
					log.Printf("收到来自 %s 的音频轨道", user)
					for {
						_, _, err := track.ReadRTP()
						if err != nil {
							return
						}
					}
				})

				offer, err := rtc.PeerConnection.CreateOffer(nil)
				if err != nil {
					log.Printf("创建Offer失败: %v", err)
					continue
				}

				if err = rtc.PeerConnection.SetLocalDescription(offer); err != nil {
					log.Printf("设置本地描述失败: %v", err)
					continue
				}

				ui.WSConnection.SendSignal("offer", user, offer.SDP)
			}
		}
	}
	ui.Contacts = filtered
	ui.ContactList.Refresh()
	if len(ui.Contacts) > 0 {
		ui.StatusLabel.SetText("在线用户: " + strings.Join(ui.Contacts, ", "))
	} else {
		ui.StatusLabel.SetText("没有其他在线用户")
	}
}
func (ui *UIClient) startVoiceCapture() {
	stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Audio: func(c *mediadevices.MediaTrackConstraints) {
			c.DeviceID = prop.String("default")
		},
	})
	if err != nil {
		log.Printf("获取麦克风失败: %v", err)
		return
	}
	ui.MediaStream = stream
	for _, contact := range ui.Contacts {
		if rtc, exists := ui.WebRTC[contact]; exists {
			audioTracks := stream.GetAudioTracks()
			if len(audioTracks) == 0 {
				log.Println("找不到音频轨道")
				continue
			}

			if _, err := rtc.PeerConnection.AddTrack(audioTracks[0]); err != nil {
				log.Printf("添加音频轨道失败: %v", err)
			}
		}
	}
	for ui.VoiceActive {
		time.Sleep(time.Second)
	}
	if ui.MediaStream != nil {
		ui.MediaStream.Close()
		ui.MediaStream = nil
	}
}
