package support

import (
	"errors"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/ipc"
)

// IPC hata kodları (IPC.md "Hata kodları").
const (
	CodeInvalidParams      = ipc.CodeInvalidParams
	CodeInternal           = ipc.CodeInternal
	CodeCodeInvalid        = -32001
	CodeRateLimited        = -32002
	CodeEnded              = -32003
	CodeUnavailable        = -32004
	CodeNetwork            = -32005
	CodePermissionRequired = -32006
	CodeUnsupported        = -32007
	CodeInvalidState       = -32008
	CodeServer             = -32009
	CodeUnauthorized       = -32010
	CodeConflict           = -32011
	CodeCaptureFailed      = -32012
	CodeLiveKit            = -32013
)

var kinds = map[int]string{
	CodeInvalidParams:      "INVALID_PARAMS",
	CodeInternal:           "INTERNAL",
	CodeCodeInvalid:        "CODE_INVALID",
	CodeRateLimited:        "RATE_LIMITED",
	CodeEnded:              "ENDED",
	CodeUnavailable:        "UNAVAILABLE",
	CodeNetwork:            "NETWORK",
	CodePermissionRequired: "PERMISSION_REQUIRED",
	CodeUnsupported:        "UNSUPPORTED",
	CodeInvalidState:       "INVALID_STATE",
	CodeServer:             "SERVER",
	CodeUnauthorized:       "UNAUTHORIZED",
	CodeConflict:           "CONFLICT",
	CodeCaptureFailed:      "CAPTURE_FAILED",
	CodeLiveKit:            "LIVEKIT",
}

// Kind, bir kodun IPC kind adıdır.
func Kind(code int) string {
	if k, ok := kinds[code]; ok {
		return k
	}
	return "INTERNAL"
}

// errorData, ipc.Error.Data alanıdır.
type errorData struct {
	Kind       string `json:"kind"`
	Permission string `json:"permission,omitempty"`
}

func rpcError(code int, msg string) *ipc.Error {
	return &ipc.Error{Code: code, Message: msg, Data: errorData{Kind: Kind(code)}}
}

func permissionError(perm, msg string) *ipc.Error {
	return &ipc.Error{Code: CodePermissionRequired, Message: msg, Data: errorData{Kind: Kind(CodePermissionRequired), Permission: perm}}
}

// Müşteriye gösterilebilir iletiler.
const (
	msgInvalidState = "Bu işlem şu anda yapılamaz."
	msgNetwork      = "Sunucuya ulaşılamıyor. Bağlantınızı denetleyin."
	msgServer       = "Sunucu şu anda yanıt veremiyor. Biraz sonra yeniden deneyin."
	msgLiveKit      = "Görüntü sunucusuna bağlanılamadı."
	msgCapture      = "Ekran paylaşımı başlatılamadı."
)

// fromAPI, API hatasını IPC hatasına çevirir.
func fromAPI(err error) *ipc.Error {
	var ie *ipc.Error
	if errors.As(err, &ie) {
		return ie
	}
	e, ok := api.AsError(err)
	if !ok {
		return rpcError(CodeInternal, "Beklenmeyen bir hata oldu.")
	}
	if e.Network {
		return rpcError(CodeNetwork, msgNetwork)
	}
	switch e.Status {
	case 400, 422:
		return rpcError(CodeInvalidParams, "İstek geçersiz.")
	case 401:
		return rpcError(CodeUnauthorized, "Oturum doğrulanamadı.")
	case 403:
		return rpcError(CodeUnavailable, "Uzaktan destek bu işletme için şu anda kullanılamıyor.")
	case 404:
		return rpcError(CodeCodeInvalid, "Kod geçersiz ya da süresi dolmuş. Temsilcinizden yeni bir kod isteyin.")
	case 409:
		return rpcError(CodeConflict, "İşlem oturumun şu anki durumuyla çakışıyor.")
	case 410:
		return rpcError(CodeEnded, "Bu oturum sona erdi.")
	case 429:
		return rpcError(CodeRateLimited, "Çok fazla deneme yapıldı. Biraz bekleyip yeniden deneyin.")
	}
	return rpcError(CodeServer, msgServer)
}

// ErrorEvent, error olayının biçimidir.
type ErrorEvent struct {
	Code    int    `json:"code"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

func errorEvent(e *ipc.Error, fatal bool) ErrorEvent {
	return ErrorEvent{Code: e.Code, Kind: Kind(e.Code), Message: e.Message, Fatal: fatal}
}
