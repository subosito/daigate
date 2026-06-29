package wire

import (
	"github.com/subosito/daigate/adaptersdk"
	"github.com/subosito/daigate/adaptersdk/handler"
	"github.com/subosito/daigate/catalog"
)

func lookupHandler[T any](adapterMap, protocolMap map[string]T, t catalog.Target) (T, bool) {
	if t.Adapter != "" {
		h, ok := adapterMap[t.Adapter]
		return h, ok
	}
	h, ok := protocolMap[t.Protocol]
	return h, ok
}

func lookupChat(reg *adaptersdk.Registry, t catalog.Target) (handler.Chat, bool) {
	return lookupHandler(reg.ChatAdapters, reg.ChatHandlers, t)
}

func lookupEmbed(reg *adaptersdk.Registry, t catalog.Target) (handler.Embed, bool) {
	return lookupHandler(reg.EmbedAdapters, reg.EmbedHandlers, t)
}

func lookupImage(reg *adaptersdk.Registry, t catalog.Target) (handler.Image, bool) {
	return lookupHandler(reg.ImageAdapters, reg.ImageHandlers, t)
}

func lookupSpeech(reg *adaptersdk.Registry, t catalog.Target) (handler.Speech, bool) {
	return lookupHandler(reg.SpeechAdapters, reg.SpeechHandlers, t)
}

func lookupVideo(reg *adaptersdk.Registry, t catalog.Target) (handler.Video, bool) {
	return lookupHandler(reg.VideoAdapters, reg.VideoHandlers, t)
}