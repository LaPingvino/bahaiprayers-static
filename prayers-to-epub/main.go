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
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"text/template"
	"unicode"
)

const APILINK = "https://BahaiPrayers.net/api/prayer/"

var Local string = APILINK

type Author int

var TMPLPRAYERBOOK = template.Must(template.New("markdown").Parse(`% Bahá'í Prayers in many languages
% from bahaiprayers.net
% via devotional.gq

{{$cl := .CrossLink}}
{{$ll := .Languages}}
{{range $code, $name := $ll}}
- [{{$name}}](#{{$code}})
{{end}}

{{range $lang, $prayers := .ByLanguage}}
<a id="{{$lang}}"></a>

## {{index $ll $lang}}

{{range $cat, $discard := .ByCategory}}
[{{html $cat}}](#{{$lang}}-{{urlquery $cat}})
{{end}}

{{range $cat, $prayer := .ByCategory}}
<a id="{{$lang}}-{{urlquery $cat}}"></a> 

### {{$cat}}

{{range $prayer}}
<a id="{{.PrayerCode}}-{{$lang}}"></a> 
{{html .Text}}

-- {{.Author}}

{{.PrayerCode}} {{range (index $cl .PrayerCode)}}{{if not (eq .LanguageCode $lang)}}«[{{.LanguageName}}](#{{.PrayerCode}}-{{.LanguageCode}})» {{end}}{{end}}

----

{{end}}
{{end}}
{{end}}
`))

// Printablize returns a string with every character that is not unicode.IsGraphic removed
func Printablize(s string) string {
	var b []rune
	for _, r := range s {
		if unicode.IsGraphic(r) {
			b = append(b, r)
		}
	}
	return string(b)
}

func (a Author) String() string {
	if a > 0 && a < 4 {
		return []string{"Báb", "Bahá'u'lláh", "Abdu'l-Bahá"}[a-1]
	}
	return "Unknown author"
}

type Prayer struct {
	Id            int
	Title         string
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

type PrayerBook struct {
	LanguageName string
	LanguageCode string
	ByCategory   map[string][]Prayer
}

type PrayerBooks struct {
	ByLanguage map[string]PrayerBook
	CrossLink  map[string][]Prayer
	Languages  map[string]string
}

func FillPrayerBooks(prayers []Prayer, crossLink map[string][]Prayer) PrayerBooks {
	var books PrayerBooks
	books.ByLanguage = make(map[string]PrayerBook)
	for _, p := range prayers {
		if _, ok := books.ByLanguage[p.LanguageCode]; !ok {
			books.ByLanguage[p.LanguageCode] = PrayerBook{
				LanguageName: p.LanguageName,
				LanguageCode: p.LanguageCode,
				ByCategory:   make(map[string][]Prayer),
			}
		}
		books.ByLanguage[p.LanguageCode].ByCategory[p.Category] = append(books.ByLanguage[p.LanguageCode].ByCategory[p.Category], p)
	}
	books.CrossLink = crossLink
	books.Languages = make(map[string]string)
	// Fill the languages map: language code -> language name
	for _, p := range prayers {
		books.Languages[p.LanguageCode] = p.LanguageName
	}
	return books
}

func ProgressBar(p, max int, part string) {
	// Go to the start of the line
	fmt.Print("\r")
	// Print a progress bar
	fmt.Print("[")
	if max < 1 {
		max = 1
	}
	for i := 0; i < p*100/max; i++ {
		fmt.Print("=")
	}
	for i := p * 100 / max; i < 100; i++ {
		fmt.Print(" ")
	}
	fmt.Print("]")
	// Print the percentage
	fmt.Printf(" %3.2f%% (%s)          ", float32(p)*100/float32(max), part)
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
		defer f.Close()
		if err != nil {
			panic(err.Error())
		}
		defer f.Close()
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
		defer f.Close()
		if err != nil {
			panic(err.Error())
		}
		defer f.Close()
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

func PrayerName(prayerCode string) string {
	// Read the name from the name.list file
	// Each first field is the prayer code, everything that follows is the name
	f, err := os.Open("rel/name.list")
	defer f.Close()
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

// SavePrayerBook saves a prayer book. It's like SavePrayer but it doesn't take path elements.
func SavePrayerBook(books PrayerBooks) string {
	// create file in temp directory
	f, err := ioutil.TempFile("", "prayerbook*.md")
	if err != nil {
		panic(err.Error())
	}
	if err = TMPLPRAYERBOOK.Execute(f, books); err != nil {
		panic(err)
	}
	f.Close()
	return f.Name()
}

type Prayerfile struct {
	Prayers []Prayer
}

func main() {
	// Define command line flags
	// -d output directory (default: prayer)
	// -o output sqlite file (default: prayer.db)
	// -l list of languages (default: all)
	// the language list consists of language codes separated by commas
	// -dir output to files? (default: false)
	// -db output to sqlite? (default: false)
	// -s show bpn prayers? (default: false)
	var outputFile string
	var langs string
	flag.StringVar(&outputFile, "o", "prayerbook.epub", "Output epub file")
	flag.StringVar(&langs, "l", "all", "List of languages")
	flag.Parse()

	prayerMap := make(map[string]Prayerfile) // map of language code to prayer file
	fmt.Println("Starting download...")
	for i, v := range languages() {
		lang, name, _ := Language(v)
		b := GetFile("prayersystembylanguage?html=true&languageid=" + strconv.Itoa(v))
		// Parse the file
		var prayers = Prayerfile{}
		err := json.Unmarshal(b, &prayers)
		if err != nil {
			panic(err)
		}
		ProgressBar(i, len(languages()), lang)
		for i, prayer := range prayers.Prayers {
			prayer.Title = PrayerName(PrayerCode(prayer.Id, true)) + " in " + name
			prayer.LanguageId = v
			prayer.LanguageCode = lang
			prayer.LanguageName = name
			prayer.PrayerCode = PrayerCode(prayer.Id, true)
			prayer.PrayerCodeTag = PrayerCode(prayer.Id, false)
			prayer.Text = Printablize(prayer.Text)
			prayers.Prayers[i] = prayer
		}
		// Save the prayers to the map
		prayerMap[lang] = prayers
	}
	fmt.Println()
	fmt.Println("Saving prayers to file...")
	// Put all prayers in a slice and in a cross-reference map per prayer code
	prayers := make([]Prayer, 0)
	crosslink := make(map[string][]Prayer)
	for _, pf := range prayerMap {
		for _, prayer := range pf.Prayers {
			prayers = append(prayers, prayer)
			crosslink[prayer.PrayerCode] = append(crosslink[prayer.PrayerCode], prayer)
		}
	}
	// Fill the prayer books
	prayerbook := FillPrayerBooks(prayers, crosslink)
	// Save prayer book per language
	tempfile := SavePrayerBook(prayerbook) // Save the prayer book
	// Convert prayerbook.md to prayerbook.epub via pandoc
	cmd := exec.Command("pandoc", tempfile, "-o", outputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println("An error occured. The most likely cause is that pandoc is not installed.")
		panic(err)
	}
	fmt.Println()
}
