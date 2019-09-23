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
	"strconv"
	"strings"
)

var dayOfWeekNames = []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"}

var instituteNames = map[string]string{
	"iep":    "ИЭП",
	"itht":   "ИТХТ",
	"rts":    "РТС",
	"kbisp":  "КБиСП",
	"ikbsp":  "КБиСП",
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

type NewSubject struct {
	DayOfWeek  int    `json:"day_of_week"`
	PairNumber string `json:"pair_number"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	IsEven     bool   `json:"is_even"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Lecturer   string `json:"lecturer"`
	Class      string `json:"class"`
}

type DayOfWeek struct {
	Name     string     `json:"name"`
	Subjects []*Subject `json:"subjects"`
}

type Group struct {
	Name      string        `json:"name"`
	StudyYear string        `json:"study_year"`
	Institute string        `json:"institute"`
	RootGroup string        `json:"root_group"`
	Subjects  []*NewSubject `json:"subjects"`
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

var t = make(chan *Group)

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

func GroupsCollector(t <-chan *Group, col *mgo.Collection) {
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

func DownloadScl(url, year string, t chan<- *Group) {
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
		log.Println("Cant find institute for ", url)
		return
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

	ParseFile(b, year, instituteName, url[strings.LastIndex(url, "/")+1:])
}

func ParseFile(b []byte, year string, instituteName string, fileName string) {
	var file, e = xlsx.OpenBinary(b)
	if e != nil {
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
		var dayOfWeekCellId int

		for ri, row := range sheet.Rows {
			for ci, cell := range row.Cells {
				text := cell.String()

				if text == "День недели" {
					dayOfWeekCellId = ci
				}

				if ri == 1 {
					groups := group.FindAllString(text, -1)
					for _, gp := range groups {

						resGroup := &Group{
							StudyYear: year,
							Name:      gp,
							Institute: instituteName,
							RootGroup: gp[:len(gp)-6],
						}

						var subs []*NewSubject
						for dayIndex := 3; dayIndex < len(sheet.Rows); dayIndex++ {
							subjectName := sheet.Cell(dayIndex, ci).String()

							if subjectName != "" {

								subj := new(NewSubject)

								subj.Name = subjectName

								dayOfWeek := getNearestCellString(sheet, dayIndex, dayOfWeekCellId)
								subj.DayOfWeek = getIndexOfWeekDay(dayOfWeek)

								subj.PairNumber = getNearestCellString(sheet, dayIndex, dayOfWeekCellId+1)

								subj.StartTime = strings.Replace(getNearestCellString(sheet, dayIndex, dayOfWeekCellId+2), "-", ":", 1)
								subj.EndTime = strings.Replace(getNearestCellString(sheet, dayIndex, dayOfWeekCellId+3), "-", ":", 1)

								subj.IsEven = getNearestCellString(sheet, dayIndex, dayOfWeekCellId+4) == "II"

								sbType := strings.TrimSpace(sheet.Cell(dayIndex, ci+1).String())
								if sbType == "" {
									sbType = "-"
								}
								subj.Type = sbType

								lecturer := strings.TrimSpace(sheet.Cell(dayIndex, ci+2).String())
								if lecturer == "" {
									lecturer = "-"
								}
								subj.Lecturer = lecturer

								class := strings.TrimSpace(sheet.Cell(dayIndex, ci+3).String())
								if class == "" {
									class = "-"
								}
								subj.Class = class
								subs = append(subs, subj)
							}
						}
						resGroup.Subjects = subs
						t <- resGroup
					}
				}
			}
		}
	}
	fmt.Println(completed, fileName, "completed")
}

func getNearestCellString(sheet *xlsx.Sheet, rowId int, cellId int) string {
	for i := rowId; i > 0; i-- {
		text := sheet.Cell(i, cellId).String()
		if text != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func getIndexOfWeekDay(expectedWdName string) int {
	for i, wdName := range dayOfWeekNames {
		if strings.EqualFold(expectedWdName, wdName) {
			return i
		}
	}
	return -1
}

func FilterSubjects(subjects []*NewSubject, weeksLeft int64) []*NewSubject {
	pt := regexp.MustCompile(`((кр|кр\s*\.)?\s*([0-9]+(?:,|\s*[0-9]+)*)+\s*(н)+\s*)`)
	var result []*NewSubject
	isEven := weeksLeft%2 == 0

	for _, subj := range subjects {

		groups := pt.FindStringSubmatch(subj.Name)
		if subj.IsEven != isEven {
			continue
		}
		if groups == nil && subj.Name != "" && subj.Name != "-" {
			result = append(result, subj)
			continue
		}
		if groups != nil && groups[3] != "" {
			weeks := strings.Split(groups[3], ",")
			var isInWeeksRange bool
			for _, week := range weeks {
				if week == strconv.FormatInt(weeksLeft, 10) {
					isInWeeksRange = true
				}
			}

			subj.Name = pt.ReplaceAllString(subj.Name, "")
			if isInWeeksRange && groups[2] == "" {
				result = append(result, subj)
			}
			if !isInWeeksRange && groups[2] != "" {
				result = append(result, subj)
			}
		}
	}
	return result
}
