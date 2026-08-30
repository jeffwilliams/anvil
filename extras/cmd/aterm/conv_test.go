package main

import (
	"testing"

	"github.com/jeffwilliams/anvil/api/go/anvil"
	"github.com/jeffwilliams/terminal"
	"github.com/stretchr/testify/assert"
)

func TestConvertColorsToTints(t *testing.T) {

	tests := []struct {
		name   string
		colors []color
		tints  []anvil.Tint
	}{
		{
			"empty",
			[]color{},
			[]anvil.Tint{},
		},
		{
			"one",
			[]color{
				{
					fg: terminal.White,
					bg: terminal.Black,
				},
			},
			[]anvil.Tint{
				{
					Start: 0,
					End:   1,
					Tint:  "#FFFFFF/#000000",
				},
			},
		},
		{
			"run of one color",
			[]color{
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
			},
			[]anvil.Tint{
				{
					Start: 0,
					End:   3,
					Tint:  "#FFFFFF/#000000",
				},
			},
		},
		{
			"start with default",
			[]color{
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
			},
			[]anvil.Tint{
				{
					Start: 2,
					End:   5,
					Tint:  "#FFFFFF/#000000",
				},
			},
		},
		{
			"end with default",
			[]color{
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
			},
			[]anvil.Tint{
				{
					Start: 0,
					End:   3,
					Tint:  "#FFFFFF/#000000",
				},
			},
		},
		{
			"start and end with default",
			[]color{
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
			},
			[]anvil.Tint{
				{
					Start: 2,
					End:   5,
					Tint:  "#FFFFFF/#000000",
				},
			},
		},
		{
			"default mix",
			[]color{
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.Red, terminal.DefaultBG},
				{terminal.Red, terminal.DefaultBG},
			},
			[]anvil.Tint{
				{
					Start: 2,
					End:   4,
					Tint:  "#FFFFFF/#000000",
				},
				{
					Start: 6,
					End:   8,
					Tint:  "#800000",
				},
			},
		},
		{
			"non-default mix",
			[]color{
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.White, terminal.Black},
				{terminal.White, terminal.Black},
				{terminal.Red, terminal.DefaultBG},
				{terminal.Red, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.LightGreen},
				{terminal.DefaultFG, terminal.LightGreen},
				{terminal.DefaultFG, terminal.DefaultBG},
				{terminal.DefaultFG, terminal.DefaultBG},
			},
			[]anvil.Tint{
				{
					Start: 2,
					End:   4,
					Tint:  "#FFFFFF/#000000",
				},
				{
					Start: 4,
					End:   6,
					Tint:  "#800000",
				},
				{
					Start: 6,
					End:   8,
					Tint:  "/#00FF00",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c Converter

			tints := c.convertColorsToTints(tc.colors)
			assert.Equal(t, tc.tints, tints)

		})
	}

}
