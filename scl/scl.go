package scl

import (
	"fmt"
	"github.com/tealeg/xlsx"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var dayOfWeekNames = []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"}

var instituteNames = map[string]string{
	"iep":    "ИЭП",
	"itht":   "ИТХТ",
	"rts":    "РТС",
	"kbisp":  "КБиСП",
	"kib":    "КИБ",
	"it":     "ИТ",
	"integu": "ИНТЕГУ",
	"fti":    "ФТИ",
}

type Subject struct {
	Name     string `json:"name"`
	Class    string `json:"class"`
	Lecturer string `json:"lecturer"`
	Type     string `json:"type"`
}

type DayOfWeek struct {
	Name     string     `json:"name"`
	Subjects []*Subject `json:"subjects"`
}

func generateWeek() []*DayOfWeek {
	var week []*DayOfWeek
	for _, name := range dayOfWeekNames {
		dof := DayOfWeek{}
		dof.Name = name
		week = append(week, &dof)
	}
	return week
}

type Group struct {
	Name string       `json:"gp_name"`
	Week []*DayOfWeek `json:"week"`
}

type TestGroup struct {
	Name      string       `json:"name"`
	StudyYear string       `json:"study_year"`
	Institute string       `json:"institute"`
	RootGroup string       `json:"root_group"`
	Week      []*DayOfWeek `json:"week"`
}

var indexes = map[int]int{
	3:  0,
	15: 1,
	27: 2,
	39: 3,
	51: 4,
	63: 5,
}

var MIREA_SCL_URL = "https://www.mirea.ru/education/schedule-main/schedule/"

var completed = 0
var sTime int64 = 0

var t = make(chan *TestGroup)

func StartGroupCollector(col *mgo.Collection) {
	go GroupsCollector(t, col)
}

func getUrlsFromSite() []string {
	res, err := http.Get(MIREA_SCL_URL)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	sclUrl, err := regexp.Compile(`http.*\.xlsx`)
	if err != nil {
		panic(err)
	}
	body, _ := ioutil.ReadAll(res.Body)
	text := string(body)

	urls := sclUrl.FindAllString(text, -1)
	return urls
}

func UpdateFormSite(instName, year string) {
	urls := getUrlsFromSite()
	found := false
	for _, url := range urls {
		if strings.Contains(strings.ToLower(url), instName) {
			found = true
			go DownloadScl(url, year, t)
		}
	}
	if !found {
		fmt.Println("Institute", instName, "not found")
	}
}

func UpdateAllFromSite(year string) {
	urls := getUrlsFromSite()
	for _, url := range urls {
		go DownloadScl(url, year, t)
	}
}

func GroupsCollector(t <-chan *TestGroup, col *mgo.Collection) {
	for {
		group := <-t

		info, err := col.Upsert(bson.M{"name": group.Name}, group)
		if err != nil {
			log.Fatal(err)
		}

		if info.Updated > 0 {
			log.Println("Group", group.Name, "was updated")
		} else if info.Matched > 0 {
			log.Println("No changes in", group.Name, "group")
		}
	}
}

func DownloadScl(url, year string, t chan<- *TestGroup) {
	startDt := time.Now().UnixNano()

	var instituteName string
	var found = false
	for k, v := range instituteNames {
		if strings.Contains(strings.ToLower(url), k) {
			instituteName = v
			found = true
			break
		}
	}

	if !found {
		instituteName = ""
	}

	res, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
		return
	}

	var file, e = xlsx.OpenBinary(b)
	if err != nil {
		log.Println(e)
		return
	}

	group, err := regexp.Compile(`[А-Яа-я]{4}-\d{2}-\d{2}`)
	if err != nil {
		log.Println(err)
		return
	}

	if file == nil || file.Sheets == nil {
		return
	}

	for _, sheet := range file.Sheets {

		for ri, row := range sheet.Rows {
			for ci, cell := range row.Cells {
				text := cell.String()

				if ri == 1 {
					groups := group.FindAllString(text, -1)
					for _, gp := range groups {

						resGroup := &TestGroup{
							StudyYear: year,
							Name:      gp,
							Week:      generateWeek(),
							Institute: instituteName,
							RootGroup: gp[:len(gp)-6],
						}

						for wd_index := 3; wd_index < 75; wd_index += 12 {
							var subs []*Subject
							for i := 0; i < 12; i++ {
								sb_name := sheet.Cell(i+wd_index, ci).String()
								if sb_name == "" {
									sb_name = "-"
								}
								sb_class := sheet.Cell(i+wd_index, ci+3).String()
								if sb_class == "" {
									sb_class = "-"
								}
								sb_lecturer := sheet.Cell(i+wd_index, ci+2).String()
								if sb_lecturer == "" {
									sb_lecturer = "-"
								}
								sb_type := sheet.Cell(i+wd_index, ci+1).String()
								if sb_type == "" {
									sb_type = "-"
								}
								s := new(Subject)
								s.Name = sb_name
								s.Class = sb_class
								s.Lecturer = sb_lecturer
								s.Type = sb_type
								subs = append(subs, s)
							}
							resGroup.Week[indexes[wd_index]].Subjects = subs
						}
						t <- resGroup
					}
				}
			}
		}
	}

	cDate := (time.Now().UnixNano() - startDt) / (1000 * 1000)
	sTime += cDate
	fmt.Println(completed, url[strings.LastIndex(url, "/")+1:], "	completed: ", cDate)

}
