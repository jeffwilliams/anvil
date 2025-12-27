package draw

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func fourFloats(f1, f2, f3, f4 float32) [4]interface{} {
	return [4]interface{}{f1, f2, f3, f4}
}

func twoFloats(f1, f2 float32) [4]interface{} {
	return [4]interface{}{f1, f2, nil, nil}
}

func TestScan(t *testing.T) {
	tests := []struct {
		name         string
		instructions string
		ops          []Op
		err          error
	}{
		{
			name:         "one call",
			instructions: "color(1,2,3,4)",
			ops: []Op{{
				Code: Color,
				Args: fourFloats(1, 2, 3, 4),
			}},
			err: nil,
		},
		{
			name:         "two calls",
			instructions: "color(1,2,3,4); begin",
			ops: []Op{
				{
					Code: Color,
					Args: fourFloats(1, 2, 3, 4),
				},
				{
					Code: Begin,
				}},
			err: nil,
		},
		{
			name:         "three calls no spaces",
			instructions: "color(1,2,3,4);begin;move(-3,0)",
			ops: []Op{
				{
					Code: Color,
					Args: fourFloats(1, 2, 3, 4),
				},
				{
					Code: Begin,
				},
				{
					Code: Move,
					Args: twoFloats(-3, 0),
				},
			},
			err: nil,
		},
		{
			name: "three calls newlines",
			instructions: `
color(1,2,3,4);
			begin;
			move(-3,0)`,
			ops: []Op{
				{
					Code: Color,
					Args: fourFloats(1, 2, 3, 4),
				},
				{
					Code: Begin,
				},
				{
					Code: Move,
					Args: twoFloats(-3, 0),
				},
			},
			err: nil,
		},
		{
			name:         "three calls with spaces",
			instructions: "color (1, 2, 3, 4); begin  ; move ( -3 , 0 )",
			ops: []Op{
				{
					Code: Color,
					Args: fourFloats(1, 2, 3, 4),
				},
				{
					Code: Begin,
				},
				{
					Code: Move,
					Args: twoFloats(-3, 0),
				},
			},
			err: nil,
		},
		{
			name:         "all opcodes",
			instructions: "begin; move; line; close; color;",
			ops: []Op{
				{
					Code: Begin,
				},
				{
					Code: Move,
				},
				{
					Code: Line,
				},
				{
					Code: Close,
				},
				{
					Code: Color,
				},
			},
			err: nil,
		},
		{
			name:         "extra semicolons",
			instructions: "begin; ; ;",
			ops: []Op{
				{
					Code: Begin,
				},
			},
			err: nil,
		},
		{
			name:         "invalid opcode",
			instructions: "boop;",
			ops:          nil,
			err:          fmt.Errorf("parse error at offset 5: invalid operation 'boop'"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			ops, err := Parse(tc.instructions)
			assert.Equal(t, tc.err, err)
			assert.Equal(t, tc.ops, ops)
		})
	}
}
