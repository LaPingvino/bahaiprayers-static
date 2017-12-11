package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

const APILINK = "https://BahaiPrayers.net/api/prayer/"

var Local string = APILINK

func main() {
	for _, name := range []string{"Languages", "RidvanLanguages", "AqdasLanguages", "HiddenLanguages", "GleaningLanguages", "PMLanguages", "TabLanguages", "IqanLanguages", "SaqLanguages", "DaysRememberLanguages"} {
		print(name + " ")
		for _, v := range languages(name) {
			print(v)
			print(" ")
		}
		println(";")
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

func languages(name string) []int {
	type Lang []struct{ Id int }
	b := GetFile(name)
	var temp = new(Lang)
	json.Unmarshal(b, temp)
	r := make([]int, len(*temp))
	for i, v := range *temp {
		r[i] = v.Id
	}
	return r
}
