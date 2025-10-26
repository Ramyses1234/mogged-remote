package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"image"
	"image/jpeg"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
	"github.com/google/uuid"
	"golang.design/x/clipboard"

	"syscall"
	"unsafe"
)

// Windows API
var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
	procSetCursorPos = user32.NewProc("SetCursorPos")
	procMouseEvent   = user32.NewProc("mouse_event")
	procKeybdEvent   = user32.NewProc("keybd_event")
	procVkKeyScanA   = user32.NewProc("VkKeyScanA")
)

const (
	MOUSEEVENTF_MOVE      = 0x0001
	MOUSEEVENTF_LEFTDOWN  = 0x0002
	MOUSEEVENTF_LEFTUP    = 0x0004
	MOUSEEVENTF_RIGHTDOWN = 0x0008
	MOUSEEVENTF_RIGHTUP   = 0x0010
	MOUSEEVENTF_WHEEL     = 0x0800
)

func MessageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	tx, _ := syscall.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(tx)), uintptr(unsafe.Pointer(t)), 0)
}

func setCursorPos(x, y int) {
	procSetCursorPos.Call(uintptr(x), uintptr(y))
}

func mouseClick(button string) {
	if button == "right" {
		procMouseEvent.Call(uintptr(MOUSEEVENTF_RIGHTDOWN), 0, 0, 0, 0)
		time.Sleep(8 * time.Millisecond)
		procMouseEvent.Call(uintptr(MOUSEEVENTF_RIGHTUP), 0, 0, 0, 0)
	} else {
		procMouseEvent.Call(uintptr(MOUSEEVENTF_LEFTDOWN), 0, 0, 0, 0)
		time.Sleep(8 * time.Millisecond)
		procMouseEvent.Call(uintptr(MOUSEEVENTF_LEFTUP), 0, 0, 0, 0)
	}
}

func mouseWheel(delta int32) {
	procMouseEvent.Call(uintptr(MOUSEEVENTF_WHEEL), 0, 0, uintptr(delta), 0)
}

func keyTapFromName(name string) {
	switch name {
	case "Shift":
		procKeybdEvent.Call(0x10, 0, 0, 0)
		procKeybdEvent.Call(0x10, 0, 2, 0)
		return
	case "Control":
		procKeybdEvent.Call(0x11, 0, 0, 0)
		procKeybdEvent.Call(0x11, 0, 2, 0)
		return
	case "Alt":
		procKeybdEvent.Call(0x12, 0, 0, 0)
		procKeybdEvent.Call(0x12, 0, 2, 0)
		return
	}
	if len(name) > 0 {
		b := name[0]
		ret, _, _ := procVkKeyScanA.Call(uintptr(b))
		vk := byte(ret & 0xff)
		if vk != 0xff {
			procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
			time.Sleep(6 * time.Millisecond)
			procKeybdEvent.Call(uintptr(vk), 0, 2, 0)
		}
	}
}

type registerMsg struct {
	Type string `json:"type"`
	Id   string `json:"id,omitempty"`
}

type controlMsg struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

func captureAllMonitors() ([]byte, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, nil
	}
	var full image.Rectangle
	for i := 0; i < n; i++ {
		full = full.Union(screenshot.GetDisplayBounds(i))
	}
	img := image.NewRGBA(full)
	for i := 0; i < n; i++ {
		b := screenshot.GetDisplayBounds(i)
		bi, err := screenshot.CaptureRect(b)
		if err != nil {
			return nil, err
		}
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				img.Set(x, y, bi.At(x-b.Min.X, y-b.Min.Y))
			}
		}
	}
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 60}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func main() {
	defaultServer := "ws://YOUR_VPS_HOST:3000"
	server := flag.String("server", defaultServer, "WebSocket server URL")
	fps := flag.Int("fps", 8, "frames per second")
	flag.Parse()

	if err := clipboard.Init(); err != nil {
		log.Println("clipboard init failed:", err)
	}

	u, err := url.Parse(*server)
	if err != nil {
		log.Fatal("invalid server url:", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	hostID := uuid.New().String()
	_ = conn.WriteJSON(registerMsg{Type: "register_host", Id: hostID})

	log.Println("Host running. ID:", hostID)
	clipboard.Write(clipboard.FmtText, []byte(hostID))
	go MessageBox("Host ID", "ID copiado para o clipboard:\n"+hostID)

	go func() {
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("read error:", err)
				os.Exit(0)
			}
			if mt == websocket.TextMessage {
				var cm controlMsg
				if err := json.Unmarshal(msg, &cm); err == nil {
					if cm.Type == "control" && cm.Payload != nil {
						if t, _ := cm.Payload["type"].(string); t == "mouse" {
							act, _ := cm.Payload["action"].(string)
							if act == "move" {
								xf, _ := cm.Payload["x"].(float64)
								yf, _ := cm.Payload["y"].(float64)
								setCursorPos(int(xf), int(yf))
							} else if act == "click" {
								btn, _ := cm.Payload["button"].(string)
								mouseClick(btn)
							} else if act == "wheel" {
								if d, ok := cm.Payload["delta"].(float64); ok {
									mouseWheel(int32(d))
								}
							}
						} else if t == "key" {
							k, _ := cm.Payload["key"].(string)
							if k != "" {
								keyTapFromName(k)
							}
						}
					}
				}
			}
		}
	}()

	interval := time.Duration(1000/(*fps)) * time.Millisecond
	for {
		start := time.Now()
		frame, err := captureAllMonitors()
		if err != nil {
			log.Println("capture error:", err)
			time.Sleep(time.Second)
			continue
		}
		if frame != nil {
			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				log.Println("write frame error:", err)
				return
			}
		}
		elapsed := time.Since(start)
		if elapsed < interval {
			time.Sleep(interval - elapsed)
		}
	}
}
