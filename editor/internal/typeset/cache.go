package typeset

import (
	"gioui.org/text"
	"github.com/jeffwilliams/anvil/editor/internal/cache"
	"github.com/timtadh/data-structures/hashtable"
	hashtypes "github.com/timtadh/data-structures/types"
)

var layoutCaches = cache.New[layoutCacheKey, *hashtable.Hash](10)

func layoutCacheForConstraints(constraints Constraints) *hashtable.Hash {
	k := layoutCacheKey{
		constraints.FontSize,
		constraints.FontFaceId,
		constraints.WrapWidth,
		constraints.TabStopInterval,
	}

	entry := layoutCaches.Get(k)
	var cache *hashtable.Hash
	if entry == nil {
		cache = addNewLayoutCache(k)
	} else {
		cache = entry.Val
	}

	return cache
}

func addNewLayoutCache(k layoutCacheKey) *hashtable.Hash {
	cache := hashtable.NewHashTable(200)
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
	shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	(*t)[fontFace] = shaper
	return shaper
}

var textShapers = make(textShaperCache)

func GetTextShaper(fontFace text.FontFace) *text.Shaper {
	return textShapers.get(fontFace)
}

type runeSliceKey []rune

func (k runeSliceKey) Hash() int {
	hash := len(k)

	for _, r := range k {
		hash = hash*314159 + int(r)
	}

	return hash
}

func (k runeSliceKey) Less(b hashtypes.Sortable) bool {
	var min int
	eqlen := false
	var ksmaller bool

	bslice := b.(runeSliceKey)

	if len(k) == len(bslice) {
		eqlen = true
		min = len(k)
	} else {
		min = len(k)
		ksmaller = true
		if len(bslice) < min {
			min = len(bslice)
			ksmaller = false
		}
	}

	for i := 0; i < min; i++ {
		if k[i] < bslice[i] {
			return true
		} else if k[i] > bslice[i] {
			return false
		}
	}

	if eqlen {
		return false
	}

	return ksmaller
}

func (k runeSliceKey) Equals(b hashtypes.Equatable) bool {
	bslice := b.(runeSliceKey)

	if len(k) != len(bslice) {
		return false
	}

	for i, r := range k {
		if r != bslice[i] {
			return false
		}
	}

	return true
}
