package typeset

import (
	"gioui.org/text"
	"github.com/jeffwilliams/anvil/editor/internal/cache"
)

var layoutCaches = cache.New[layoutCacheKey, cache.Cache[string, []Line]](10)

func layoutCacheForConstraints(constraints Constraints) cache.Cache[string, []Line] {
	k := layoutCacheKey{
		constraints.FontSize,
		constraints.FontFaceId,
		constraints.WrapWidth,
		constraints.TabStopInterval,
	}

	entry := layoutCaches.Get(k)
	var cache cache.Cache[string, []Line]
	if entry == nil {
		cache = addNewLayoutCache(k)
	} else {
		cache = entry.Val
	}

	return cache
}

func addNewLayoutCache(k layoutCacheKey) cache.Cache[string, []Line] {
	cache := cache.New[string, []Line](200)
	layoutCaches.Set(k, cache)
	return cache
}

type layoutCacheKey struct {
	FontSize        int
	FaceId          string
	WrapWidth       int
	TabStopInterval int
}

type textShaperCache map[text.FontFace]*text.Shaper

func (t *textShaperCache) get(fontFace text.FontFace) *text.Shaper {
	shaper, ok := (*t)[fontFace]
	if ok {
		return shaper
	}

	collection := []text.FontFace{fontFace}
	shaper = text.NewShaper(text.WithCollection(collection))
	(*t)[fontFace] = shaper
	return shaper
}

var textShapers = make(textShaperCache)

func GetTextShaper(fontFace text.FontFace) *text.Shaper {
	return textShapers.get(fontFace)
}
