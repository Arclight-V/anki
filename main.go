package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Arclight-V/laroussefr/conjugation"
	"github.com/Arclight-V/laroussefr/definition"
	"github.com/atselvan/ankiconnect"
)

const (
	basicTypeInAnswer      = "Basic (type in the answer)"
	basicAndReversedFrench = "Basic (and reversed card french)"
	defenition             = "French Defenition"
	defenitiodAndRevers    = "Basic (and reversed card (Word/Transcription/PartOfSpeach/Audio)"
	indicatifPresent       = "French Conjugation: INDICATIF Présent"
	imperatifPresent       = "French Conjugation: IMPÉRATIF Présent"
)

var re = regexp.MustCompile(`^(j'|je|tu|il, elle|nous|vous|ils, elles)\s*`)

type File struct {
	Verbs      DecksContainer `json:"verbs"`
	Definition DecksContainer `json:"definition"`
	Phrases    DecksContainer `json:"phrases"`
}

type DecksContainer struct {
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

	for _, deck := range decks.Phrases.Decks {
		name := deck.Name

		for _, phrase := range deck.Words {
			{
				note := ankiconnect.Note{
					DeckName:  name,
					ModelName: basicTypeInAnswer,
					Fields: ankiconnect.Fields{
						// "Front":   phrase, -To avoid duplicates, the translation must be manually entered into the anki.
						"Front": phrase,
						"Back":  phrase,
					},
				}
				restErr := client.Notes.Add(note)
				if restErr != nil {
					log.Println(restErr, basicTypeInAnswer)
				}
			}

			{
				note := ankiconnect.Note{
					DeckName:  name,
					ModelName: basicAndReversedFrench,
					Fields: ankiconnect.Fields{
						// "Rubric":   phrase, -To avoid duplicates, the translation must be manually entered into the anki.
						"Front": phrase,
						"Back":  phrase,
					},
				}
				restErr := client.Notes.Add(note)
				if restErr != nil {
					log.Println(restErr, basicAndReversedFrench)
				}
			}

		}
	}

	for _, deck := range decks.Definition.Decks {
		name := deck.Name

		for _, word := range deck.Words {
			result, err := definition.New(word)
			if err != nil {
				panic(err)
			}

			for _, h := range result.Header {
				var filename string
				{

					data, tmpName, err := downloadMP3(h.Audio)
					if err != nil {
						log.Fatalf("download error: %v", err)
					}

					filename = stableMP3Name(tmpName, data)

					_, errAnki := client.Media.StoreMediaFile(filename, base64.StdEncoding.EncodeToString(data))
					if errAnki != nil {
						log.Printf("store error: %v", errAnki)
					}
					log.Printf("stored media: %s", filename)
				}

				{
					note := ankiconnect.Note{
						DeckName:  name,
						ModelName: defenition,
						Fields: ankiconnect.Fields{
							// "Rubric":   h.Texte, -To avoid duplicates, the translation must be manually entered into the anki.
							"Rubric":   h.Texte,
							"Question": "{{c1::" + h.Texte + "}}",
							"Image":    h.Type,
							"Audio":    "[sound:" + filename + "]",
						},
					}
					restErr := client.Notes.Add(note)
					if restErr != nil {
						log.Println(restErr, defenition)
					}
				}

				{
					note := ankiconnect.Note{
						DeckName:  name,
						ModelName: defenitiodAndRevers,
						Fields: ankiconnect.Fields{
							"Front-Expression":   h.Texte,
							"Front-PartOfSpeech": h.Type,
							"Front-Audio":        "[sound:" + filename + "]",
							"Back":               "-",
						},
					}
					restErr := client.Notes.Add(note)
					if restErr != nil {
						log.Println(restErr, defenitiodAndRevers)
					}
				}

			}
		}
	}

	for _, deck := range decks.Verbs.Decks {
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

func httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func downloadMP3(rawURL string) ([]byte, string, error) {
	resp, err := httpClient().Get(rawURL)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("bad status: %s", resp.Status)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	u, err := url.Parse(rawURL)
	filename := "audio.mp3"
	if err == nil {
		base := path.Base(u.Path)
		if base != "" && strings.Contains(base, ".") {
			filename = base
		}
	}

	if !strings.HasSuffix(strings.ToLower(filename), ".mp3") {
		filename += ".mp3"
	}
	return buf, filename, nil
}

func stableMP3Name(orig string, data []byte) string {
	sum := sha1.Sum(data)
	hash := fmt.Sprintf("%x", sum[:8]) // короткий префикс
	// очищаем имя и прицепляем хэш
	orig = strings.ReplaceAll(orig, " ", "_")
	orig = strings.ReplaceAll(orig, "/", "_")
	orig = strings.ReplaceAll(orig, "\\", "_")
	// убираем повторное .mp3 на всякий
	orig = strings.TrimSuffix(orig, ".mp3")
	return fmt.Sprintf("%s_%s.mp3", orig, hash)
}
