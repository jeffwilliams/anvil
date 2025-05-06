package fuzzy

import (
	"fmt"
	"strings"
	"unicode"
)

// CalcScore calculates a score that measures how well
// the terms match the candidate string. Each term is individually
// matched against the candidate using Peter H. Seller's algorithm [1] and then
// the final score is the average of the scores of the terms.
// Most of the code is ported from the Node package "fast-fuzzy" by Ethan Rutherford
// [1] https://pdfs.semanticscholar.org/0517/aa6d420f66f74bd4b281e2ed0e2021f3d359.pdf
//
// The returned score is between 0 and 1.
// We modify the Sellers score sligtly, such that if not all the runes of a term are
// found in the candidate, that term is given a score of 0. As well, if a term
// begins or ends a space-separated word it is given a bonus, and if it _both_ begins
// and ends a space separated word it is given a further bonus.
func CalcScore(terms []string, candidate string, s CaseSensitivity) Score {
	terms, candidate = toLowerIfNeeded(terms, candidate, s)

	var avg float32
	var ms Score
	ms.Start = make([]int, len(terms))
	ms.End = make([]int, len(terms))

	c := []rune(candidate)
	pencount := 0
	for i, t := range terms {
		runes := []rune(t)
		s, penalized := scoreRuneSliceWithPenalty(runes, c)
		applyBonuses(runes, c, &s)
		avg += s.score
		ms.Start[i] = s.start
		ms.End[i] = s.end
		if penalized {
			pencount += 1
		}
	}

	if pencount == len(terms) {
		ms.Score = 0
		ms.Start = ms.Start[:0]
		ms.End = ms.End[:0]
		return ms
	}

	avg /= float32(len(terms))
	ms.Score = avg
	return ms
}

func toLowerIfNeeded(terms []string, candidate string, s CaseSensitivity) (nterms []string, ncandidate string) {
	if s == CaseSensitive {
		return terms, candidate
	}

	for i, v := range terms {
		terms[i] = strings.ToLower(v)
		nterms = terms
	}

	ncandidate = strings.ToLower(candidate)
	return
}

type CaseSensitivity int

const (
	CaseSensitive CaseSensitivity = iota
	CaseInsensitive
)

// Score is the measurement of how well a term matches a candidate.
type Score struct {
	// Score is the actual numeric score. It is between 0 and 1, with higher being better.
	Score float32
	// Start and End mark the position of the best match for each respective term. term[i] starts
	// at Start[i] and ends at End[i].
	Start, End []int
}

func calcSingleTermScore(term, candidate string) singleScore {
	return scoreRuneSlice([]rune(term), []rune(candidate))
}

type singleScore struct {
	score      float32
	start, end int
}

func (s singleScore) String() string {
	return fmt.Sprintf("(score: %.2f indes: %d end: %d)", s.score, s.start, s.end)
}

// scoreRuneSliceWithPenalty is the same as scoreRuneSlice, but changes the score to 0
// if candidate does not contain all the runes in term.
func scoreRuneSliceWithPenalty(term, candidate []rune) (score singleScore, penalized bool) {
	score = scoreRuneSlice(term, candidate)

	if !runesInOneAreInTwo(term, candidate) {
		score.score = 0
	}

	return
}

func applyBonuses(term, candidate []rune, score *singleScore) {

	if score.score == 0 {
		return
	}

	// If the term starts or ends a word, then apply a bonus
	sb := score.start == 0 || unicode.IsSpace(candidate[score.start-1])
	eb := score.end == len(candidate)-1 || unicode.IsSpace(candidate[score.end+1])
	if sb || eb {
		score.score += 0.2
		if sb && eb {
			score.score += 0.1
		}
	}

	score.score = score.score / 1.3
}

func scoreRuneSlice(term, candidate []rune) singleScore {

	rows := initSellersRows(len(term)+1, len(candidate)+1)
	for j := 0; j < len(candidate); j++ {
		levenshtein(term, candidate, rows, j)
		//printRows(rows)
	}

	scoreVal, scoreIndex := getSellersScore(rows, len(candidate)+1)
	start, end := walkBack(rows, scoreIndex)

	return singleScore{
		scoreVal, start, end,
	}
}

func initSellersRows(rowCount, columnCount int) [][]int {
	rows := make([][]int, rowCount)
	for i := range rows {
		rows[i] = make([]int, columnCount)
		rows[i][0] = i
	}

	return rows
}

// runtime complexity: O(mn) where m and n are the lengths of term and candidate, respectively
// Note: this method only runs on a single column
func levenshtein(term, candidate []rune, rows [][]int, j int) {
	for i := 0; i < len(term); i++ {
		levCore(term, candidate, rows, i, j)
	}
}

// the content of the innermost loop of levenshtein
func levCore(term, candidate []rune, rows [][]int, i, j int) {
	rowA := rows[i]
	rowB := rows[i+1]

	cost := 0
	if term[i] != candidate[j] {
		cost = 1
	}

	min := rowB[j] + 1 // insertion
	m := rowA[j+1] + 1
	if m < min {
		min = m // deletion
	}
	m = rowA[j] + cost
	if m < min {
		min = m // substitution
	}

	rowB[j+1] = min
}

func getSellersScore(rows [][]int, length int) (score float32, index int) {
	// search term was empty string, return perfect score
	if len(rows) == 1 {
		score = 1
		index = 0
		return
	}

	lastRow := rows[len(rows)-1]
	minValue := lastRow[0]
	minIndex := 0
	for i := 1; i < length; i++ {
		val := lastRow[i]

		if val < minValue {
			minValue = val
			minIndex = i
		}
	}

	//fmt.Println("scoring. miyyn: ", minValue)

	score = 1.0 - (float32(minValue) / float32(len(rows)-1))
	index = minIndex
	return
}

func walkBack(rows [][]int, scoreIndex int) (start, end int) {
	if scoreIndex == 0 {
		return
	}

	start = scoreIndex
	for i := len(rows) - 2; i > 0 && start > 1; i-- {
		row := rows[i]
		if row[start] >= row[start-1] {
			start--
		}
	}

	start = start - 1
	end = scoreIndex - 1
	return
}

func printRows(rows [][]int) {
	for i, r := range rows {
		fmt.Printf("%d: ", i)
		for _, c := range r {
			fmt.Printf("%d ", c)

		}
		fmt.Println()
	}
}

func runesInOneAreInTwo(one, two []rune) bool {
outer:
	for _, r := range one {
		for _, r2 := range two {
			if r == r2 {
				continue outer
			}
		}
		return false
	}
	return true
}
