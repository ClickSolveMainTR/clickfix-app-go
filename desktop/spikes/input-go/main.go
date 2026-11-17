// Command input-go, CF-235 keşfinde tüm adaylarla kullanılan girdi enjeksiyonu
// denemesidir: LiveKit "control" konusundaki fare/klavye mesajlarını alır ve
// robotgo ile ana ekrana uygular. Yalnız keşif içindir.
package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>

// prompt=1 ise sistem izin penceresini açar.
static int cf_ax_trusted(int prompt) {
	if (!prompt) return AXIsProcessTrusted() ? 1 : 0;
	const void *k[] = {kAXTrustedCheckOptionPrompt};
	const void *v[] = {kCFBooleanTrue};
	CFDictionaryRef o = CFDictionaryCreate(NULL, k, v, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int r = AXIsProcessTrustedWithOptions(o) ? 1 : 0;
	CFRelease(o);
	return r;
}

// Ana ekran boyutu, nokta cinsinden (robotgo da nokta kullanır).
static void cf_main_bounds(double *x, double *y, double *w, double *h) {
	CGRect b = CGDisplayBounds(CGMainDisplayID());
	*x = b.origin.x; *y = b.origin.y; *w = b.size.width; *h = b.size.height;
}

// Tuş basılıyken hareket: robotgo yalnız MouseMoved gönderir, sürükleme için
// LeftMouseDragged/RightMouseDragged gerekir.
static void cf_drag(double x, double y, int right) {
	CGEventType t = right ? kCGEventRightMouseDragged : kCGEventLeftMouseDragged;
	CGEventRef e = CGEventCreateMouseEvent(NULL, t, CGPointMake(x, y), right ? kCGMouseButtonRight : kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}
*/
import "C"

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-vgo/robotgo"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

type msg struct {
	T    string  `json:"t"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	B    int     `json:"b"`
	Key  string  `json:"key"`
	Code string  `json:"code"`
	TS   int64   `json:"ts"`
}

// DOM KeyboardEvent.key -> robotgo tuş adı.
var named = map[string]string{
	"Enter": "enter", "Backspace": "backspace", "Tab": "tab", "Escape": "esc",
	"ArrowUp": "up", "ArrowDown": "down", "ArrowLeft": "left", "ArrowRight": "right",
	" ": "space", "Delete": "delete", "Home": "home", "End": "end",
	"PageUp": "pageup", "PageDown": "pagedown",
}

type tokenResp struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	Room  string `json:"room"`
}

func fetchToken(harness, identity string) (tokenResp, error) {
	var t tokenResp
	resp, err := http.Get(harness + "/token?identity=" + url.QueryEscape(identity) + "&publish=1")
	if err != nil {
		return t, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return t, fmt.Errorf("harness %s", resp.Status)
	}
	return t, json.NewDecoder(resp.Body).Decode(&t)
}

type injector struct {
	dry    bool
	ox, oy float64 // ana ekran kökeni (nokta)
	w, h   float64 // ana ekran boyutu (nokta)
	held   int     // -1 yok, 0 sol, 2 sağ
}

func (in *injector) pt(nx, ny float64) (float64, float64) {
	nx = min(max(nx, 0), 1)
	ny = min(max(ny, 0), 1)
	return in.ox + nx*(in.w-1), in.oy + ny*(in.h-1)
}

func button(b int) string {
	switch b {
	case 2:
		return "right"
	case 1:
		return "center"
	}
	return "left"
}

func (in *injector) handle(m msg, recv time.Time) {
	lat := ""
	if m.TS > 0 {
		lat = fmt.Sprintf(" gecikme %d ms", recv.UnixMilli()-m.TS)
	}
	switch m.T {
	case "move":
		x, y := in.pt(m.X, m.Y)
		if !in.dry {
			if in.held == 0 || in.held == 2 {
				C.cf_drag(C.double(x), C.double(y), C.int(btoi(in.held == 2)))
			} else {
				_ = robotgo.Move(int(x), int(y))
			}
		}
		// Hareket çok sık; ayrıntı yalnız dry modda.
		if in.dry {
			fmt.Printf("move %.0f,%.0f\n", x, y)
		}
	case "down", "up":
		x, y := in.pt(m.X, m.Y)
		var err error
		if !in.dry {
			_ = robotgo.Move(int(x), int(y))
			err = robotgo.Toggle(button(m.B), m.T)
		}
		if m.T == "down" {
			in.held = m.B
		} else {
			in.held = -1
		}
		fmt.Printf("%s %s %.0f,%.0f%s%s\n", m.T, button(m.B), x, y, lat, errText(err))
	case "key":
		err := in.key(m.Key)
		fmt.Printf("key %q (%s)%s%s\n", m.Key, m.Code, lat, errText(err))
	default:
		fmt.Printf("bilinmeyen mesaj: %+v\n", m)
	}
}

func (in *injector) key(k string) error {
	if in.dry {
		return nil
	}
	if n, ok := named[k]; ok {
		return robotgo.KeyTap(n)
	}
	if utf8.RuneCountInString(k) != 1 {
		return fmt.Errorf("eşlenmemiş tuş")
	}
	r, _ := utf8.DecodeRuneInString(k)
	switch {
	case r < 128 && unicode.IsLower(r), r < 128 && unicode.IsDigit(r):
		return robotgo.KeyTap(k)
	case r < 128 && unicode.IsUpper(r):
		return robotgo.KeyTap(strings.ToLower(k), "shift")
	default:
		// Noktalama, Türkçe harfler vb.: unicode olarak yaz.
		robotgo.TypeStr(k)
		return nil
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func errText(err error) string {
	if err != nil {
		return " HATA: " + err.Error()
	}
	return ""
}

func main() {
	harness := flag.String("harness", "http://127.0.0.1:7890", "harness adresi")
	identity := flag.String("identity", "customer-input", "LiveKit kimliği")
	from := flag.String("from", "agent", "yalnız bu kimlikten gelen mesajları uygula (boş = herkes)")
	dry := flag.Bool("dry", false, "enjekte etme, yalnız logla (izin penceresi açmaz)")
	smoke := flag.Bool("smoke", false, "odaya bağlanıp çık (izin denetlemez)")
	dur := flag.Duration("duration", 0, "bu süre sonra dur (0 = Ctrl+C'ye kadar)")
	flag.Parse()

	tok, err := fetchToken(*harness, *identity)
	if err != nil {
		fail("token alınamadı: %v", err)
	}

	if !*smoke {
		// dry modda pencere açmadan yalnız durumu oku.
		if C.cf_ax_trusted(C.int(btoi(!*dry))) == 1 {
			fmt.Println("erişilebilirlik izni: var")
		} else if *dry {
			fmt.Println("erişilebilirlik izni: yok (dry mod, sorun değil)")
		} else {
			fmt.Fprintln(os.Stderr, "Erişilebilirlik izni yok. Sistem Ayarları > Gizlilik ve Güvenlik > Erişilebilirlik'te bu uygulamaya (terminal) izin verin ve tekrar çalıştırın.")
			os.Exit(2)
		}
	}

	in := &injector{dry: *dry, held: -1}
	var ox, oy, w, h C.double
	C.cf_main_bounds(&ox, &oy, &w, &h)
	in.ox, in.oy, in.w, in.h = float64(ox), float64(oy), float64(w), float64(h)
	robotgo.KeySleep = 0
	robotgo.MouseSleep = 0

	type evt struct {
		m msg
		t time.Time
	}
	events := make(chan evt, 256)
	cb := &lksdk.RoomCallback{ParticipantCallback: lksdk.ParticipantCallback{
		OnDataPacket: func(d lksdk.DataPacket, p lksdk.DataReceiveParams) {
			u, ok := d.(*lksdk.UserDataPacket)
			if !ok || u.Topic != "control" {
				return
			}
			if *from != "" && p.SenderIdentity != *from {
				return
			}
			var m msg
			if err := json.Unmarshal(u.Payload, &m); err != nil {
				fmt.Println("geçersiz mesaj:", err)
				return
			}
			select {
			case events <- evt{m, time.Now()}:
			default:
				fmt.Println("kuyruk dolu, mesaj atıldı")
			}
		},
	}}

	room, err := lksdk.ConnectToRoomWithToken(tok.URL, tok.Token, cb, lksdk.WithAutoSubscribe(false))
	if err != nil {
		fail("odaya bağlanılamadı: %v", err)
	}
	defer room.Disconnect()
	fmt.Printf("%s odasına %s olarak bağlandı; ana ekran %.0fx%.0f nokta; dry=%v\n", room.Name(), *identity, in.w, in.h, *dry)
	if *smoke {
		return
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	var stop <-chan time.Time
	if *dur > 0 {
		stop = time.After(*dur)
	}
	for {
		select {
		case e := <-events:
			in.handle(e.m, e.t)
		case <-sig:
			fmt.Println("durduruluyor")
			return
		case <-stop:
			fmt.Println("süre doldu")
			return
		}
	}
}

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", a...)
	os.Exit(1)
}
