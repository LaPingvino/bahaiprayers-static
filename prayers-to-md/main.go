//A tool to download all Bahá'í prayers on bahaiprayers.net through the API
// and save them locally in Markdown format for usage in Devotional.gq and maybe other sites
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
	"encoding/csv"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"text/template"
	"fmt"
)

const APILINK = "https://BahaiPrayers.net/api/prayer/"

var Local string = APILINK

type Author int

var TMPLOUTPUT = template.Must(template.New("markdown").Parse(`+++
title = '{{.Title}}'
tags = ['lang-{{.LanguageCode}}', '{{.PrayerCode}}']
+++
{{.Text}}

-- {{.Author}}
`))

func (a Author) String() string {
	if a > 0 && a < 4 {
		return []string{"Báb", "Bahá'u'lláh", "Abdu'l-Bahá"}[a-1]
	}
	return "Unknown author"
}

type Prayer struct {
	Id           int
	Title        string
	LanguageCode string
	PrayerCode   string
	Author       Author `json:"AuthorId"`
	LanguageId   int
	Text         string
	Category     string `json:"FirstTagName"`
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

var LCOnce sync.Once
var LC = map[int][]string{}
var PCOnce sync.Once
var PC = map[int]string{}

func Language(lang int) (code string, name string, rtl string) {
	LCOnce.Do(func() {
		f, err := os.Open("rel/lang.csv")
		if err != nil {
			panic(err.Error())
		}
		c := csv.NewReader(f)
		c.FieldsPerRecord = 7
		ls, err := c.ReadAll()
		if err != nil {
			panic(err.Error())
		}
		for _, l := range ls[1:] {
			lc, err := strconv.Atoi(l[0])
			if err != nil {
				panic(err.Error())
			}
			LC[lc] = l
		}
	})
	code = strconv.Itoa(lang)
	if LC[lang] != nil {
		code = LC[lang][1]
		name = LC[lang][3]
		rtl = LC[lang][6]
	}
	return
}

func PrayerCode(prayer int) (code string) {
	PCOnce.Do(func() {
		f, err := os.Open("rel/code.list")
		if err != nil {
			panic(err.Error())
		}
		c := csv.NewReader(f)
		c.FieldsPerRecord = -1
		ps, err := c.ReadAll()
		if err != nil {
			panic(err.Error())
		}
		for _, p := range ps {
			for _, bpns := range p[1:] {
				bpn, err := strconv.Atoi(bpns)
				if err != nil {
					panic(err.Error())
				}
				PC[bpn] = p[0]
			}
		}
	})
	code = "bpn" + strconv.Itoa(prayer)
	if PC[prayer] != "" {
		code = PC[prayer]
	}
	return
}

var dirbase = "prayer/"

func main() {
	type Prayerfile struct {
		Prayers []Prayer
	}
	for _, v := range languages() {
		log.Printf("Language %d", v)
		b := GetFile("prayersystembylanguage?html=false&languageid=" + strconv.Itoa(v))
		var prayers = Prayerfile{}
		err := json.Unmarshal(b, &prayers)
		if err != nil {
			panic(err)
		}
		log.Printf("%#v", prayers)
		for _, prayer := range prayers.Prayers {
			log.Printf("Prayer %d", prayer.Id)
			var lname, rtl string
			prayer.LanguageCode, lname, rtl = Language(v)
			os.MkdirAll(dirbase + prayer.LanguageCode, os.ModePerm)
			f, err := os.Create(dirbase + prayer.LanguageCode + "/_index.md")
			if err != nil {
				panic(err)
			}
			fmt.Fprintln(f, `---
title: "`+lname+`"
rtl: "`+rtl+`"
---`)
			f.Close()
			prayer.PrayerCode = PrayerCode(prayer.Id)
			prayer.Title = "Prayer " + prayer.PrayerCode + " in " + lname
			dir := dirbase + prayer.LanguageCode + "/" + prayer.PrayerCode
			os.Mkdir(dir, os.ModePerm)
			f, err = os.Create(dir + "/_index.md")
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
