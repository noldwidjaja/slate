package arango

import (
	"fmt"
	"strconv"
	"strings"
)

type ArangoQuery struct {
	collection string
	query      string
	filterArgs map[string]interface{}
	joins      []string
	withs      []string
	sortField  string
	sortOrder  string
	offset     int
	limit      int
}

func SubQuery(collection string) *ArangoQuery {
	return &ArangoQuery{
		collection: collection,
	}
}

func (r *ArangoQuery) Where(column string, operator string, value interface{}) *ArangoQuery {
	if strings.Contains(column, ".") {
		r.query += " FILTER " + column + " " + operator + " @" + r.collection + "_" + column
	} else {
		r.query += " FILTER " + r.collection + "." + column + " " + operator + " @" + r.collection + "_" + column
	}

	if r.filterArgs == nil {
		r.filterArgs = make(map[string]interface{})
	}

	if strings.ToUpper(operator) == "LIKE" {
		value = "%" + value.(string) + "%"
	}

	r.filterArgs[r.collection+"_"+column] = value

	return r
}

func (r *ArangoQuery) WhereOr(column string, operator string, value interface{}) *ArangoQuery {
	if strings.Contains(column, ".") {
		r.query += " OR " + column + " " + operator + " @" + r.collection + "_" + column
	} else {
		r.query += " OR " + r.collection + "." + column + " " + operator + " @" + r.collection + "_" + column
	}

	if r.filterArgs == nil {
		r.filterArgs = make(map[string]interface{})
	}

	if strings.ToUpper(operator) == "LIKE" {
		value = "%" + value.(string) + "%"
	}

	r.filterArgs[r.collection+"_"+column] = value

	return r
}

func (r *ArangoQuery) WhereColumn(column string, operator string, value string) *ArangoQuery {
	if strings.Contains(column, ".") {
		r.query += " OR " + column + " " + operator + " " + value
	} else {
		r.query += " OR " + r.collection + "." + column + " " + operator + " " + value
	}
	return r
}

func (r *ArangoQuery) Join(from, fromKey, To, toKey string) *ArangoQuery {
	r.query += ` FOR ` + To + ` IN ` + To + `
		FILTER ` + To + "." + toKey + "==" + from + "." + fromKey + `
	`

	r.joins = append(r.joins, To)

	return r
}

func (r *ArangoQuery) With(from, fromKey, to, toKey, alias string) *ArangoQuery {
	r.query += ` LET ` + alias + ` = (
		FOR ` + to + ` IN ` + to + `
		FILTER ` + to + "." + toKey + "==" + from + "." + fromKey

	// if len(filters) > 0 {
	// 	for _, filter := range filters {
	// 		// filter()
	// 	}
	// }

	r.query += `
		RETURN ` + to + `
	)`

	r.withs = append(r.withs, alias)

	return r
}

func (r *ArangoQuery) JoinEdge(from, fromKey, edge, alias, direction string) *ArangoQuery {
	r.query += `
		FOR ` + alias + ` IN ` + direction + " " + from + "." + fromKey + " " + edge + `
	`

	r.joins = append(r.joins, alias)

	return r
}

func (r *ArangoQuery) WithEdge(from, fromKey, edge, alias, direction string) *ArangoQuery {
	r.query += ` LET ` + alias + ` = (
		FOR ` + alias + ` IN ` + direction + " " + from + "." + fromKey + " " + edge + `
		RETURN ` + alias + ` 
	)`

	r.withs = append(r.withs, alias)

	return r
}

func (r *ArangoQuery) Offset(offset int) *ArangoQuery {
	r.offset = offset
	return r
}

func (r *ArangoQuery) Limit(limit int) *ArangoQuery {
	r.limit = limit

	return r
}

func (r *ArangoQuery) Sort(sortField, sortOrder string) *ArangoQuery {
	if strings.Contains(sortField, ".") {
		r.sortField = sortField
	} else {
		r.sortField = r.collection + "." + sortField
	}

	if sortOrder != "ASC" {
		r.sortOrder = "DESC"
	} else {
		r.sortOrder = "ASC"
	}

	return r
}

func (r *ArangoQuery) Get(request interface{}) error {

	r.query, r.filterArgs = r.Raw()

	return nil
}

func (r *ArangoQuery) Raw() (string, map[string]interface{}) {
	var (
		returnData string
		limitQuery string
		sortQuery  string
	)

	returnData = "MERGE("
	for _, join := range r.joins {
		returnData += join + ", "
	}

	if len(r.withs) > 0 {
		returnData += "{"
		for index, with := range r.withs {
			if index == 0 {
				returnData += with + " :" + with
			} else {
				returnData += ", " + with + " :" + with
			}
		}
		returnData += "}, "
	}

	returnData += r.collection + ")"

	if r.limit > 0 {
		limitQuery = "LIMIT " + strconv.Itoa(r.offset) + "," + strconv.Itoa(r.limit)
	}

	if r.sortField != "" {
		sortQuery = "SORT " + r.sortField + " " + r.sortOrder
	}

	rawQuery := fmt.Sprintf("FOR %s in %s %s %s %s RETURN %s",
		r.collection,
		r.collection,
		r.query,
		limitQuery,
		sortQuery,
		returnData,
	)

	args := r.filterArgs

	r.clearQuery()

	return rawQuery, args
}

func (r *ArangoQuery) clearQuery() {
	r.query = ""
	r.filterArgs = make(map[string]interface{})
	r.joins = []string{}
	r.withs = []string{}
	r.sortField = ""
	r.sortOrder = ""
	r.offset = 0
	r.limit = 0
}