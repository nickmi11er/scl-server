package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"scl-server/scl"
	"strconv"
	"strings"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func HomeRouterHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello World!")
}

func InstitutesHandler(w http.ResponseWriter, r *http.Request) {
	var institutes []string
	err := col.Find(nil).Distinct("institute", &institutes)
	if err != nil {
		fmt.Fprintf(w, "Internal server error")
		return
	}
	jInst, err := json.Marshal(institutes)
	if err == nil {
		fmt.Fprintf(w, string(jInst))
	}
}

func RootGroupsHandler(w http.ResponseWriter, r *http.Request) {
	var rootGroups []string

	inst, ok := r.URL.Query()["inst"]
	if ok && len(inst[0]) > 0 {
		err := col.Find(bson.M{"institute": inst[0]}).Distinct("rootgroup", &rootGroups)
		if err != nil {
			fmt.Fprintf(w, "Groups not found")
			return
		}
	} else {
		err := col.Find(nil).Distinct("rootgroup", &rootGroups)
		if err != nil {
			fmt.Fprintf(w, "Groups not found")
			return
		}
	}

	jRootGroups, err := json.Marshal(rootGroups)
	if err == nil {
		fmt.Fprintf(w, string(jRootGroups))
	}
}

func GroupsHandler(w http.ResponseWriter, r *http.Request) {
	var groups []string
	rGroup, ok := r.URL.Query()["rootGroup"]

	if ok && len(rGroup[0]) > 0 {
		err := col.Find(bson.M{"rootgroup": rGroup[0]}).Distinct("name", &groups)
		if err != nil {
			fmt.Fprintf(w, "Groups not found")
			return
		}
	} else {
		err := col.Find(nil).Distinct("name", &groups)
		if err != nil {
			fmt.Fprintf(w, "Groups not found")
			return
		}
	}

	jGroups, err := json.Marshal(groups)
	if err == nil {
		fmt.Fprintf(w, string(jGroups))
	}
}

func SclUpdateHandler(w http.ResponseWriter, r *http.Request) {
	year, ok := r.URL.Query()["year"]
	if !ok || len(year[0]) < 1 {
		fmt.Fprintf(w, "Url Param 'year' is missing")
		return
	}

	inst, ok := r.URL.Query()["inst"]
	if !ok || len(inst[0]) < 1 {
		scl.UpdateAllFromSite(year[0])
	} else {
		scl.UpdateFormSite(inst[0], year[0])
	}
}

func SclHandler(w http.ResponseWriter, r *http.Request) {
	year, ok := r.URL.Query()["year"]
	if !ok || len(year[0]) < 1 {
		fmt.Fprintf(w, "Url Param 'year' is missing")
		return
	}

	group, ok := r.URL.Query()["group"]
	if !ok || len(group[0]) < 1 {
		fmt.Fprintf(w, "Url Param 'group' is missing")
		return
	}

	dayOfWeek, ok := r.URL.Query()["dow"]
	if !ok || len(dayOfWeek[0]) < 1 {
		fmt.Fprintf(w, "Url Param 'dow' is missing")
		return
	}

	weeksLeft, ok := r.URL.Query()["weeksLeft"]
	if !ok || len(dayOfWeek[0]) < 1 {
		fmt.Fprintf(w, "Url Param 'weeksLeft' is missing")
		return
	}

	gp := scl.Group{}
	err := col.Find(bson.M{"name": group[0], "studyyear": year[0]}).One(&gp)
	if err != nil {
		fmt.Fprintf(w, "Schedule not found")
	} else {
		dowi, err := strconv.ParseInt(dayOfWeek[0], 10, 64)
		wl, err1 := strconv.ParseInt(weeksLeft[0], 10, 64)
		if err == nil && err1 == nil {
			subjects := gp.Subjects
			var filtered []*scl.NewSubject

			for _, subj := range subjects {
				if dowi == int64(subj.DayOfWeek) {
					filtered = append(filtered, subj)
				}
			}

			filtered = scl.FilterSubjects(filtered, wl)
			gp.Subjects = filtered
			jDay, err := json.Marshal(gp)
			if err == nil {
				fmt.Fprintf(w, string(jDay))
			}
		}
	}
}

var col *mgo.Collection

func main() {
	session, err := mgo.Dial("localhost")
	if err != nil {
		panic(err)
	}
	defer session.Close()

	db := session.DB("scl")
	col = db.C("groups")

	scl.StartGroupCollector(col)

	http.HandleFunc("/", HomeRouterHandler)
	http.HandleFunc("/scl", SclHandler)
	http.HandleFunc("/scl/update", SclUpdateHandler)
	http.HandleFunc("/institutes", InstitutesHandler)
	http.HandleFunc("/groups", GroupsHandler)
	http.HandleFunc("/rootGroups", RootGroupsHandler)
	http.HandleFunc("/scl/upfile/", SclUpfileHandler)

	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

const FileForm = `
<html>
    <head>
    <title></title>
    </head>
    <body>
        <form id="sclUpload" action="/scl/upfile" method="post" enctype="multipart/form-data" onSubmit="return setUrl();">
			File:<input type="file" name="sclfile"><br>
            Год:<input id="year" type="text" name="year"><br>
            Институт:<input id="inst" type="text" name="inst"><br>
            <input type="submit" value="Загрузить">
        </form>
    </body>
	<script type=text/javascript>
		function setUrl() {
			document.getElementById('sclUpload').action = "/scl/upfile/" + document.getElementById('year').value + "/" + document.getElementById('inst').value
		}
	</script>
</html>`

func SclUpfileHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, FileForm)
	case http.MethodPost:
		r.ParseForm()

		params := strings.Split(strings.Replace(r.URL.Path, "/scl/upfile/", "", 1), "/")

		year := params[0]
		if year == "" {
			fmt.Fprintf(w, "Url Param 'year' is missing")
			return
		}

		inst := params[1]
		if inst == "" {
			fmt.Fprintf(w, "Url Param 'inst' is missing")
			return
		}

		var Buf bytes.Buffer
		file, header, err := r.FormFile("sclfile")
		if err != nil {
			fmt.Fprintf(w, "Invalid file was supplied")
			return
		}
		defer file.Close()
		name := strings.Split(header.Filename, ".")
		if name[1] != "xlsx" {
			fmt.Fprintf(w, "File should be in .xlsx format")
			return
		}
		fmt.Printf("File name %s\n", name[0])
		io.Copy(&Buf, file)
		go scl.ParseFile(Buf.Bytes(), year, inst, name[0])
		Buf.Reset()
		return
	}
}
