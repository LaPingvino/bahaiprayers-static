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
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"unicode"

	_ "modernc.org/sqlite"
)

const APILINK = "https://BahaiPrayers.net/api/prayer/"

var Local string = APILINK

type Author int

var TMPLPRAYER = template.Must(template.New("markdown").Parse(`+++
title = "{{.Title}}"
author = "{{.Author}}"
tags = ['lang-{{.LanguageCode}}', 'prayer-{{.PrayerCodeTag}}', "author-{{.Author}}", "category-{{.ENCategory}}", "cat-{{.Category}}"]
+++
{{.Text}}

(Source category: {{.Category}})
(Bahaiprayers.net ID: {{.Id}})
`))

var TMPLPRAYERBOOK = template.Must(template.New("markdown").Parse(`+++
title = "{{.LanguageName}}"
tags = ['lang={{.LanguageCode}}', 'prayerbook']
+++
{{$cl := .CrossLink}}
{{$lc := .LanguageCode}}

{{range $cat, $discard := .ByCategory}}
[{{html $cat}}](#{{urlquery $cat}})
{{end}}

{{range $cat, $prayer := .ByCategory}}
<a id="{{urlquery $cat}}"></a> 
## {{html $cat}}
{{range $prayer}}
<a id="{{.PrayerCode}}"></a> 
{{.Text}}

-- {{.Author}}

{{.PrayerCode}} {{range (index $cl .PrayerCode)}}{{if not (eq .LanguageCode $lc)}}«[{{.LanguageName}}](../../{{.LanguageCode}}/prayers/#{{.PrayerCode}})» {{end}}{{end}}

----

{{end}}
{{end}}

`))

// Printablize returns a string with all characters that are not unicode.IsGraphic removed
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

type PrayerBook struct {
	LanguageName string
	LanguageCode string
	ByCategory   map[string][]Prayer
	CrossLink    *map[string][]Prayer
}

type PrayerBooks map[string]PrayerBook

func FillPrayerBooks(prayers []Prayer, crossLink map[string][]Prayer) PrayerBooks {
	books := make(PrayerBooks)
	for _, p := range prayers {
		if _, ok := books[p.LanguageCode]; !ok {
			books[p.LanguageCode] = PrayerBook{
				LanguageName: p.LanguageName,
				LanguageCode: p.LanguageCode,
				ByCategory:   make(map[string][]Prayer),
				CrossLink:    &crossLink,
			}
		}
		books[p.LanguageCode].ByCategory[p.Category] = append(books[p.LanguageCode].ByCategory[p.Category], p)
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
	defer f.Close()
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
			return strings.Join(c[1:], ",")
		}
	}
	return "Prayer " + prayerCode
}

func SavePrayer(dir string, prayer Prayer, path ...PathElement) {
	var showBPN bool
	// check if dir ends with /
	if dir[len(dir)-1] != '/' {
		dir += "/"
	}
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
				title = PrayerName(prayerCode)
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
	if err = TMPLPRAYER.Execute(f, prayer); err != nil {
		panic(err)
	}
	f.Close()

}

// SavePrayerBook saves a prayer book. It's like SavePrayer but it doesn't take path elements.
func SavePrayerBook(outputDir, lang string, book PrayerBook) {
	// Remove the last / from the outputDir
	if outputDir[len(outputDir)-1] == '/' {
		outputDir = outputDir[:len(outputDir)-1]
	}
	dir := outputDir + "/" + lang + "/prayers/"
	// Create the directory
	os.MkdirAll(outputDir+"/"+lang, 0755)
	os.MkdirAll(dir, 0755)
	// Create a dummy file in /lang/
	f, err := os.Create(outputDir + "/" + lang + "/_index.md")
	if err != nil {
		panic(err.Error())
	}
	f.WriteString(fmt.Sprintf(`+++
title = "%s"
+++
`, lang))
	f.Close()
	// Create the file and fill it with the prayer text
	f, err = os.Create(dir + "_index.md")
	if err != nil {
		panic(err.Error())
	}
	if err = TMPLPRAYERBOOK.Execute(f, book); err != nil {
		panic(err)
	}
	f.Close()
}

type Prayerfile struct {
	Prayers []Prayer
}

// SaveToSQLite saves the prayers to the SQLite database
func SaveToSQLite(db *sql.DB, prayermap map[string]Prayerfile) {
	// Table structure: id, prayercode, language, source, title, author, text
	// Source is https://bahaiprayers.net/Book/Single/1/6342 where 1 is to be replaced by the language id and 6342 is to be replaced by the bpn id
	// Language is the language code
	// Author is the author name
	// Title is the prayer name
	// Text is the prayer text
	// Prayercode is the prayer code from code.list

	// Create the table if it doesn't exist
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS prayers (id INTEGER PRIMARY KEY, prayercode TEXT, language TEXT, source TEXT, title TEXT, author TEXT, text TEXT)`)
	if err != nil {
		panic(err.Error())
	}
	// Delete all the rows
	_, err = db.Exec(`DELETE FROM prayers`)
	if err != nil {
		panic(err.Error())
	}
	// Count the number of prayers
	total := 0
	for _, pf := range prayermap {
		for range pf.Prayers {
			total++
		}
	}
	count := 0
	// Count prayers per language
	langCount := make(map[string]int)
	// Create a transaction
	tx, err := db.Begin()
	if err != nil {
		panic(err.Error())
	}

	// Insert all the rows
	for lang, prayerfile := range prayermap {
		for _, prayer := range prayerfile.Prayers {
			_, err = tx.Exec(`INSERT INTO prayers (prayercode, language, source, title, author, text) VALUES (?, ?, ?, ?, ?, ?)`,
				PrayerCode(prayer.Id, false),
				lang,
				"https://bahaiprayers.net/Book/Single/"+strconv.Itoa(prayer.LanguageId)+"/"+strconv.Itoa(int(prayer.Id)),
				prayer.Title,
				prayer.Author.String(),
				template.HTMLEscapeString(prayer.Text),
			)
			if err != nil {
				panic(err.Error())
			}
			count++
			langCount[lang]++
			ProgressBar(count, total, lang)
		}
	}
	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		panic(err.Error())
	}
	fmt.Println()
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
	var outputDir string
	var outputFile string
	var langs string
	var outputToFiles bool
	var outputToSQLite bool
	var showBPN bool
	var prayerBook bool
	var db *sql.DB
	flag.StringVar(&outputDir, "d", "prayer", "Output directory")
	flag.StringVar(&outputFile, "o", "prayer.db", "Output sqlite file")
	flag.StringVar(&langs, "l", "all", "List of languages")
	flag.BoolVar(&outputToFiles, "dir", false, "Output to files?")
	flag.BoolVar(&outputToSQLite, "db", false, "Output to sqlite?")
	flag.BoolVar(&showBPN, "s", false, "Show bpn prayers?")
	flag.BoolVar(&prayerBook, "book", false, "Output prayer book?")
	flag.Parse()

	// if sqlite output is set, create the database
	if outputToSQLite {
		var err error
		db, err = sql.Open("sqlite", outputFile)
		if err != nil {
			panic(err.Error())
		}
		defer db.Close()
	}

	re := regexp.MustCompile(`^(#+)([^#])`)
	sanitize := func(s string) string {
		s = strings.Replace(s, "\n", "\n\n", -1)
		s = re.ReplaceAllString(s, "##$1 $2")
		return s
	}

	prayerMap := make(map[string]Prayerfile) // map of language code to prayer file
	fmt.Println("Starting download...")
	for i, v := range languages() {
		lang, name, _ := Language(v)
		b := GetFile("prayersystembylanguage?html=false&languageid=" + strconv.Itoa(v))
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
			prayer.ENCategory = CategoryByPrayer(PrayerCode(prayer.Id, true))
			prayer.Text = Printablize(sanitize(prayer.Text))
			prayers.Prayers[i] = prayer
		}
		// Save the prayers to the map
		prayerMap[lang] = prayers
	}
	fmt.Println()
	// Save the prayers to the output directory
	if outputToFiles {
		// count the number of prayers
		total := 0
		for _, pf := range prayerMap {
			for range pf.Prayers {
				total++
			}
		}
		count := 0
		fmt.Println("Saving prayers to files...")
		if prayerBook {
			// Put a title in _index.md of the output directory
			os.MkdirAll(outputDir, 0755)
			f, err := os.Create(outputDir + "/_index.md")
			if err != nil {
				panic(err.Error())
			}
			f.WriteString("+++\n")
			f.WriteString("title = 'Prayerbooks'\n")
			f.WriteString("+++\n")
			f.WriteString("Pick your language\n")
			f.WriteString("\n")
			f.Close()
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
			prayerbooks := FillPrayerBooks(prayers, crosslink)
			// Save prayer book per language
			total := len(prayerbooks)
			for lang, prayerbook := range prayerbooks {
				SavePrayerBook(outputDir, lang, prayerbook) // Save the prayer book
				count++
				ProgressBar(count, total, lang)
			}
		} else {
			for _, prayerlist := range prayerMap {
				if showBPN {
					for _, p := range prayerlist.Prayers {
						count++
						SavePrayer(outputDir, p, ShowBPN, CategoryPathElement, PrayerCodePathElement, LanguagePathElement, BPNPathElement)
					}
				} else {
					for _, p := range prayerlist.Prayers {
						count++
						SavePrayer(outputDir, p, CategoryPathElement, PrayerCodePathElement, LanguagePathElement)
					}
				}
				ProgressBar(count, total, strconv.Itoa(count))
			}
		}
		fmt.Println()
	}
	// Save the prayers to the SQLite database
	if outputToSQLite {
		fmt.Println("Saving prayers to SQLite...")
		SaveToSQLite(db, prayerMap)
	}
}
