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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"text/template"
)

const APILINK = "https://BahaiPrayers.net/api/prayer/"

var Local string = APILINK

type Author int

var TMPLOUTPUT = template.Must(template.New("markdown").Parse(`+++
title = '{{.Title}}'
author = "{{.Author}}"
tags = ['lang-{{.LanguageCode}}', '{{.PrayerCodeTag}}', "{{.Author}}", "{{.ENCategory}}"]
+++
{{.Text}}

(Source category: {{.Category}})
(Bahaiprayers.net ID: {{.Id}})
`))

func (a Author) String() string {
	if a > 0 && a < 4 {
		return []string{"Báb", "Bahá'u'lláh", "Abdu'l-Bahá"}[a-1]
	}
	return "Unknown author"
}

type Prayer struct {
	Id            int
	Title         string
	ENCategory    string
	LanguageCode  string
	LanguageName  string
	PrayerCode    string
	PrayerCodeTag string
	Author        Author `json:"AuthorId"`
	LanguageId    int
	Text          string
	Category      string `json:"FirstTagName"`
	Tags          []struct {
		Id   int
		Name string
	}
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

func PrayerCode(prayer int, showbpn bool) (code string) {
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
	if PC[prayer] != "" {
		code = PC[prayer]
	} else if showbpn {
		code = "bpn" + strconv.Itoa(prayer)
	} else {
		code = ""
	}
	return
}

var dirbase = "prayer/"

type PathElement int

const (
	LanguagePathElement PathElement = iota
	CategoryPathElement
	NamePathElement
	PrayerCodePathElement
	AuthorPathElement
	ShowBPN
	BPNPathElement
)

func CategoryByPrayer(prayerCode string) string {
	// Read the category from the category.list file
	// Each first field is the category, everything that follows is prayer codes
	f, err := os.Open("rel/category.list")
	if err != nil {
		panic(err.Error())
	}
	c := csv.NewReader(f)
	c.FieldsPerRecord = -1
	cs, err := c.ReadAll()
	if err != nil {
		panic(err.Error())
	}
	for _, c := range cs {
		for _, p := range c[1:] {
			if p == prayerCode {
				return c[0]
			}
		}
	}
	return "unsorted"
}

func PrayerName(prayerCode string) string {
	// Read the name from the name.list file
	// Each first field is the prayer code, everything that follows is the name
	f, err := os.Open("rel/name.list")
	if err != nil {
		panic(err.Error())
	}
	c := csv.NewReader(f)
	c.FieldsPerRecord = -1
	cs, err := c.ReadAll()
	if err != nil {
		panic(err.Error())
	}
	for _, c := range cs {
		if c[0] == prayerCode {
			return c[1]
		}
	}
	return "Prayer " + prayerCode
}

func SavePrayer(prayer Prayer, path ...PathElement) {
	var dir string
	var showBPN bool
	dir = dirbase
	// Iterate over the path elements and create the directory structure
	// For every but the last element, if there is no _index.md file, create it
	// and fill it with front matter
	for i, p := range path {
		var title string
		switch p {
		case LanguagePathElement:
			code, name, _ := Language(prayer.LanguageId)
			dir += code + "/"
			title = name
		case CategoryPathElement:
			dir += CategoryByPrayer(PrayerCode(prayer.Id, showBPN)) + "/"
			title = CategoryByPrayer(PrayerCode(prayer.Id, showBPN))
		case NamePathElement:
			dir += prayer.Title + "/"
			title = prayer.Title
		case PrayerCodePathElement:
			prayerCode := PrayerCode(prayer.Id, false)
			if prayerCode != "" {
				dir += prayerCode + "/"
				title = prayerCode
			} else {
				continue
			}
		case AuthorPathElement:
			dir += strconv.Itoa(int(prayer.Author)) + "/"
			title = prayer.Author.String()
		case ShowBPN:
			showBPN = true
			continue
		case BPNPathElement:
			if showBPN && PrayerCode(prayer.Id, false) == "" {
				dir += PrayerCode(prayer.Id, true) + "/"
				title = PrayerCode(prayer.Id, true)
			} else {
				continue
			}
		}
		if i < len(path)-1 {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				os.MkdirAll(dir, 0755)
				f, err := os.Create(dir + "_index.md")
				if err != nil {
					panic(err.Error())
				}
				f.WriteString(fmt.Sprintf(`+++
title = "%s"
+++
`, title))
				f.Close()
			}
		}
	}
	// Check if the file is a bpn prayer and if showBPN is true
	if !showBPN && PrayerCode(prayer.Id, showBPN) == "" {
		return
	}
	// Create the path
	os.MkdirAll(dir, 0755)
	// Create the file and fill it with the prayer text
	f, err := os.Create(dir + "_index.md")
	if err != nil {
		panic(err.Error())
	}
	if err = TMPLOUTPUT.Execute(f, prayer); err != nil {
		panic(err)
	}
	f.Close()

}

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
			lang, name, _ := Language(v)
			prayer.Title = PrayerName(PrayerCode(prayer.Id, true)) + " in " + name
			prayer.LanguageId = v
			prayer.LanguageCode = lang
			prayer.LanguageName = name
			prayer.ENCategory = CategoryByPrayer(PrayerCode(prayer.Id, true))
			SavePrayer(prayer, ShowBPN, CategoryPathElement, PrayerCodePathElement, LanguagePathElement, BPNPathElement)
		}
	}
}
