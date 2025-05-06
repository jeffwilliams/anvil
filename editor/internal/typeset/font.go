package typeset

import (
	"io/ioutil"

	"gioui.org/font/opentype"
)

func ParseTTFBytes(b []byte) (opentype.Face, error) {
	return opentype.Parse(b)
}

func ParseTTF(r Resource) (opentype.Face, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return opentype.Face{}, err
	}

	return ParseTTFBytes(b)
}

type Resource interface {
	Read([]byte) (int, error)
	ReadAt([]byte, int64) (int, error)
	Seek(int64, int) (int64, error)
}
