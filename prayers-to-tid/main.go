//A tool to download all Bahá'í prayers on bahaiprayers.net through the API
// and save them locally in the format requested by tiddlywiki to be able to
// manage them in tiddlywiki at bahaiprayers.tiddlyspot.com
//
// The bahaiprayers.net API documentation can be found at http://bahaiprayers.net/Developer
// We only use the prayer part here for now, which uses the following 3 links:
// - https://BahaiPrayers.net/api/prayer/Languages
//   to get a list of languages
// - https://BahaiPrayers.net/api/prayer/tags?languageid=1
//   to get the tags (categories) per language
// - https://BahaiPrayers.net/api/prayer/prayersystembylanguage?html=true&languageid=1
//   to get the actual prayers
package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"text/template"
)

const APILINK = "https://BahaiPrayers.net/api/prayer/"

var Local string = APILINK

type Author int

var TMPLOUTPUT = template.Must(template.New("tiddler").Parse(`title: bpn-{{.LanguageId}}-{{.Id}}
tags: [[{{.Category}}]] prayer-lang-{{.LanguageId}} Prayers [[Prayers by {{.Author}}]]

{{.Text}}
`))

func (a Author) String() string {
	if a > 0 && a < 4 {
		return string{"Báb", "Bahá'u'lláh", "Abdu'l-Bahá"}[a-1]
	}
}

type Prayer struct {
	Id         string
	Author     Author `json:"AuthorId"`
	LanguageId string
	Text       string
	Category   string `json:"FirstTagName"`
}

func GetFile(name string) []byte {
	r, err := http.Get(Local + name)
	if err != nil {
		return nil
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	return b
}

func languages() []int {
	type Lang []struct{ Id int }
	b := GetFile("Languages")
	var temp = new(Lang)
	json.Unmarshal(b, temp)
	r := make([]int, len(*temp))
	for i, v := range *temp {
		r[i] = v.Id
	}
	return r
}

func main() {
	type Prayerfile struct {
		Prayers []Prayer
	}
	for _, v := range languages() {
		b := GetFile("tags?languageid=" + strconv.Itoa(v))
		var prayers = new(Prayerfile)
		json.Unmarshal(b, prayers)
		for _, prayer := range prayers.Prayers {
			f, err := os.Create("prayerfile" + v + prayer.Id + ".tid")
			if err != nil {
				panic(err)
			}
			if err = TMPLOUTPUT.Execute(f, prayer); err != nil {
				panic(err)
			}
			f.Close()
		}
	}
}
