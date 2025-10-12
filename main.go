package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/Arclight-V/laroussefr/conjugation"
	"github.com/atselvan/ankiconnect"
)

const (
	indicatifPresent = "French Conjugation: INDICATIF Présent"
	imperatifPresent = "French Conjugation: IMPÉRATIF Présent"
)

var re = regexp.MustCompile(`^(j'|je|tu|il, elle|nous|vous|ils, elles)\s*`)

type File struct {
	Decks []Deck `json:"decks"`
}
type Deck struct {
	Name  string   `json:"name"`
	Words []string `json:"words"`
}

func ReadDecks(filePath string) (File, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, err
	}

	return f, nil
}

func PrepareToClozeNote(conjugations []string) string {
	var parts []string

	for _, c := range conjugations {
		m := re.FindString(c)
		if m != "" {
			verb := strings.TrimSpace(strings.TrimPrefix(c, m))
			parts = append(parts, fmt.Sprintf("%s {{c1::%s}}", strings.TrimSpace(m), verb))
		}
	}

	if len(parts) == 0 {
		for _, c := range conjugations {
			parts = append(parts, "{{c1::"+c+"}}")
		}
	}

	return strings.Join(parts, "<br>")
}

func main() {
	decks, err := ReadDecks("./words/verbs.json")
	if err != nil {
		panic(err)
	}

	client := ankiconnect.NewClient()

	for _, deck := range decks.Decks {
		name := deck.Name

		for _, word := range deck.Words {

			result, err := conjugation.New(word)
			if err != nil {
				panic(err)
			}

			indicatif := result.Indicatif[string(conjugation.Present)]
			imperatif := result.Imperatif[string(conjugation.Present)]

			type toSend struct {
				model    string
				question string
			}

			toSends := []toSend{
				{
					model:    indicatifPresent,
					question: PrepareToClozeNote(indicatif),
				},
				{
					model:    imperatifPresent,
					question: PrepareToClozeNote(imperatif),
				},
			}

			for _, send := range toSends {
				note := ankiconnect.Note{
					DeckName:  name,
					ModelName: send.model,
					Fields: ankiconnect.Fields{
						"Rubric":   word,
						"Question": send.question,
					},
				}
				restErr := client.Notes.Add(note)
				if restErr != nil {
					log.Println(restErr, send.model)

				}
			}

		}
	}

	// print all Words defined on this page
}
