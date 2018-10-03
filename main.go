package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"log"
	"net/http"
	"scl_preparator/scl"
	"strconv"
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
		err := col.Find(bson.M{"institute":inst[0]}).Distinct("rootgroup", &rootGroups)
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
		err := col.Find(bson.M{"rootgroup":rGroup[0]}).Distinct("name", &groups)
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

	gp := scl.TestGroup{}
	err := col.Find(bson.M{"name": group[0], "studyyear": year[0]}).One(&gp)
	if err != nil {
		fmt.Fprintf(w, "Schedule not found")
	} else {
		dowi, err := strconv.ParseInt(dayOfWeek[0], 10, 64)
		if err == nil {
			day := gp.Week[dowi]
			jDay, err := json.Marshal(day)
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

	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
