package arango

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/arangodb/go-driver"
	"github.com/noldwidjaja/slate/helper"
)

type (
	TraversalDirection string
)

const (
	INBOUND  TraversalDirection = "INBOUND"
	OUTBOUND TraversalDirection = "OUTBOUND"
	ANY      TraversalDirection = "ANY"
)

type arangoQueryTraversal struct {
	enabled   bool
	direction TraversalDirection
	sourceId  string
	withEdge  bool
}

type ArangoQuery struct {
	collection string
	traversal  arangoQueryTraversal
	query      string
	filterArgs map[string]interface{}
	joins      []*ArangoQuery
	withs      []*ArangoQuery
	returns    string
	sortField  string
	sortOrder  string
	offset     int
	limit      int
	first      bool
	alias      string
	outeralias string
	raw        bool
	ArangoDB   ArangoDB
}

func NewQuery(collection string, db ArangoDB) *ArangoQuery {
	return &ArangoQuery{
		collection: collection,
		alias:      collection,
		ArangoDB:   db,
		outeralias: collection,
	}
}

func SubQuery(collection string) *ArangoQuery {
	return &ArangoQuery{
		collection: collection,
		alias:      collection,
		outeralias: collection,
	}
}

func SubQueryWithAlias(collection string, alias string) *ArangoQuery {
	return &ArangoQuery{
		collection: collection,
		alias:      alias,
		outeralias: collection,
	}
}

func Raw(query string) *ArangoQuery {
	return &ArangoQuery{
		raw:   true,
		query: query,
	}
}

/***************************************
			Private Function
****************************************/

func (r *ArangoQuery) getArgKey(argKey string) string {
	var key string

	if r.filterArgs == nil {
		key += fmt.Sprintf("%s%v", argKey, 1)
	} else {
		key += fmt.Sprintf("%s%v", argKey, len(r.filterArgs)+1)
	}

	return key
}

func (r *ArangoQuery) where(column string, operator string, value interface{}) *ArangoQuery {
	replacer := strings.NewReplacer("(", "_", ")", "", ".", "_")
	argKey := r.getArgKey(replacer.Replace(r.alias + "_" + column))
	if strings.Contains(column, ".") || helper.IsAggregates(column) {
		r.query += " FILTER " + column + " " + operator + " @" + argKey
	} else {
		r.query += " FILTER " + r.alias + "." + column + " " + operator + " @" + argKey
	}
	if r.filterArgs == nil {
		r.filterArgs = make(map[string]interface{})
	}

	if strings.ToUpper(operator) == "LIKE" {
		value = "%" + value.(string) + "%"
	}

	r.filterArgs[argKey] = value

	return r
}

func (r *ArangoQuery) clearQuery() {
	r.query = ""
	r.filterArgs = make(map[string]interface{})
	r.joins = nil
	r.withs = nil
	r.sortField = ""
	r.sortOrder = ""
	r.returns = ""
	r.offset = 0
	r.limit = 0
	r.traversal = arangoQueryTraversal{}
}

func (r *ArangoQuery) with(query *ArangoQuery, alias string) *ArangoQuery {
	q, f := query.ToQuery()
	r.query += ` LET ` + alias + ` = ( 
      ` + q + ` 
      )
   `

	r.withs = append(r.withs, query)
	r.filterArgs = helper.MergeMaps(r.filterArgs, f)
	return r
}

func (r *ArangoQuery) toQueryWithoutReturn() (string, map[string]interface{}) {
	var (
		limitQuery string
		sortQuery  string
		finalQuery string
	)

	if r.limit > 0 {
		limitQuery = fmt.Sprintf("LIMIT %s,%s", strconv.Itoa(r.offset), strconv.Itoa(r.limit))
	}

	if r.sortField != "" {
		sortQuery = fmt.Sprintf("SORT %s %s", r.sortField, r.sortOrder)
	}

	if r.traversal.enabled {
		finalQuery = fmt.Sprintf("FOR %s in %s %s %s %s %s %s ",
			r.alias,
			r.traversal.direction,
			r.traversal.sourceId,
			r.collection,
			r.query,
			limitQuery,
			sortQuery,
		)
	} else {
		finalQuery = fmt.Sprintf("FOR %s in %s %s %s %s ",
			r.alias,
			r.collection,
			r.query,
			limitQuery,
			sortQuery,
		)
	}

	args := r.filterArgs

	return finalQuery, args
}

func (r *ArangoQuery) executeQuery(c context.Context, request interface{}) error {
	ctx := driver.WithQueryCount(c)

	data, err := r.ArangoDB.DB().Query(ctx, r.query, r.filterArgs)
	r.clearQuery()
	if err != nil {
		fmt.Println(err)
		return err
	}

	defer data.Close()

	if data.Count() > 0 {
		v := reflect.Indirect(reflect.ValueOf(request))

		if v.Kind() == reflect.Slice {
			var response []interface{}
			for data.HasMore() {
				var d interface{}
				data.ReadDocument(c, &d)
				response = append(response, d)
			}

			jsonResponse, err := json.Marshal(response)
			if err != nil {
				fmt.Println(err)
			}

			err = json.Unmarshal(jsonResponse, &request)
			if err != nil {
				fmt.Println(err)
			}
			return nil
		}

		data.ReadDocument(c, &request)
		return nil
	}

	return errors.New("not found")
}

func (r *ArangoQuery) setRawQuery(query string, args map[string]interface{}) *ArangoQuery {
	r.query = query
	r.filterArgs = args

	return r
}

/************************************************
			Public Arango Functions
************************************************/

func (r *ArangoQuery) Where(param ...interface{}) *ArangoQuery {
	column := fmt.Sprintf("%v", param[0])
	operator := fmt.Sprintf("%v", param[1])

	switch len(param) {
	case 2:
		r.where(column, "==", param[1])
	case 3:
		r.where(column, operator, param[2])
	}

	return r
}

func (r *ArangoQuery) WhereOr(column string, operator string, value interface{}) *ArangoQuery {
	argKey := strings.ReplaceAll(r.alias+"_"+column, ".", "_")

	if strings.Contains(column, ".") {
		r.query += " OR " + column + " " + operator + " @" + argKey
	} else {
		r.query += " OR " + r.alias + "." + column + " " + operator + " @" + argKey
	}

	if r.filterArgs == nil {
		r.filterArgs = make(map[string]interface{})
	}

	if strings.ToUpper(operator) == "LIKE" {
		value = "%" + value.(string) + "%"
	}

	r.filterArgs[argKey] = value

	return r
}

func (r *ArangoQuery) WhereColumn(column string, operator string, value string) *ArangoQuery {
	if strings.Contains(column, ".") || strings.Contains(column, "'") {
		r.query += " FILTER " + column + " " + operator + " " + value
	} else {
		r.query += " FILTER " + r.alias + "." + column + " " + operator + " " + value
	}
	return r
}

func (r *ArangoQuery) WhereOrColumn(column string, operator string, value string) *ArangoQuery {
	if strings.Contains(column, ".") || strings.Contains(column, "'") {
		r.query += " OR " + column + " " + operator + " " + value
	} else {
		r.query += " OR " + r.alias + "." + column + " " + operator + " " + value
	}
	return r
}

func (r *ArangoQuery) WhereRaw(params string) *ArangoQuery {
	r.query += params
	return r
}

func (r *ArangoQuery) Join(query *ArangoQuery) *ArangoQuery {
	q, f := query.toQueryWithoutReturn()
	r.query += " " + q

	r.joins = append(r.joins, query)
	r.filterArgs = helper.MergeMaps(r.filterArgs, f)
	return r
}

func (r *ArangoQuery) WithOne(repo *ArangoQuery, alias string) *ArangoQuery {
	repo.outeralias = alias
	repo.first = true
	r.with(repo, alias)
	return r
}

func (r *ArangoQuery) WithMany(repo *ArangoQuery, alias string) *ArangoQuery {
	repo.outeralias = alias
	r.first = false
	r.with(repo, alias)
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
		r.sortField = r.alias + "." + sortField
	}

	if sortOrder != "ASC" {
		r.sortOrder = "DESC"
	} else {
		r.sortOrder = "ASC"
	}

	return r
}

func (r *ArangoQuery) SortRaw(sortField, sortOrder string) *ArangoQuery {
	r.sortField = sortField

	if sortOrder != "ASC" {
		r.sortOrder = "DESC"
	} else {
		r.sortOrder = "ASC"
	}

	return r
}

func (r *ArangoQuery) Traversal(source string, direction TraversalDirection, withEdge ...bool) *ArangoQuery {
	r.traversal.enabled = true
	r.traversal.direction = direction
	r.traversal.sourceId = source

	if len(withEdge) > 0 {
		if withEdge[0] {
			r.traversal.withEdge = true
		}
	}

	return r
}

func (r *ArangoQuery) Returns(returns ...string) *ArangoQuery {
	// r.returns = "MERGE("

	for index, ret := range returns {
		if strings.Contains(ret, ":") {
			r.returns += fmt.Sprintf("{%s}", ret)
		} else {
			r.returns += ret
		}

		if len(returns) != index+1 {
			r.returns += ", "
		}
	}

	if strings.Contains(r.returns, "{") || strings.Contains(r.returns, ",") {
		r.returns = fmt.Sprintf("MERGE(%s)", r.returns)
	}

	return r
}

/***********************************************
			Query Specific Functions
***********************************************/

func (r *ArangoQuery) ToQuery() (string, map[string]interface{}) {
	var (
		returnData string
		limitQuery string
		sortQuery  string
		finalQuery string
	)
	if r.raw == true {
		return r.query, r.filterArgs
	}
	if r.returns == "" {
		returnData = "MERGE(" + r.alias

		if len(r.withs) > 0 {
			returnData += ", {"
			for index, with := range r.withs {
				alias := with.outeralias
				if with.first {
					alias = fmt.Sprintf(" FIRST(%s)", alias)
				}

				if index == 0 {
					returnData += fmt.Sprintf("%s :%s", with.outeralias, alias)
				} else {
					returnData += fmt.Sprintf(", %s :%s", with.outeralias, alias)
				}
			}
			returnData += "}"
		}

		if len(r.joins) > 0 {
			for _, join := range r.joins {
				returnData += fmt.Sprintf(", %s", join.alias)
			}
		}
		returnData += ")"
	} else {
		returnData = r.returns
	}

	if r.limit > 0 {
		limitQuery = fmt.Sprintf("LIMIT %s,%s", strconv.Itoa(r.offset), strconv.Itoa(r.limit))
	}

	if r.sortField != "" {
		sortQuery = fmt.Sprintf("SORT %s %s", r.sortField, r.sortOrder)
	}

	if r.traversal.enabled {
		var collectionWithEdge string
		collectionWithEdge = r.alias
		if r.traversal.withEdge {
			collectionWithEdge = r.alias + ",edge"
			returnData = "{document:" + returnData + ", edge: edge}"
		}
		finalQuery = fmt.Sprintf("FOR %s in %s %s %s %s %s %s RETURN %s",
			collectionWithEdge,
			r.traversal.direction,
			r.traversal.sourceId,
			r.collection,
			r.query,
			sortQuery,
			limitQuery,
			returnData,
		)
	} else {
		finalQuery = fmt.Sprintf("FOR %s in %s %s %s %s RETURN %s",
			r.alias,
			r.collection,
			r.query,
			sortQuery,
			limitQuery,
			returnData,
		)
	}

	args := r.filterArgs

	return finalQuery, args
}

func (r *ArangoQuery) Get(request interface{}) error {

	r.query, r.filterArgs = r.ToQuery()

	return r.executeQuery(context.Background(), request)
}

func (r *ArangoQuery) GetWithContext(c context.Context, request interface{}) error {

	r.query, r.filterArgs = r.ToQuery()

	return r.executeQuery(c, request)
}

func (r *ArangoQuery) Count(request interface{}) error {
	var (
		returnData string
		limitQuery string
		sortQuery  string
	)

	returnData = "COLLECT WITH COUNT INTO total RETURN total"

	r.query = fmt.Sprintf("FOR %s in %s %s %s %s %s",
		r.collection,
		r.collection,
		r.query,
		limitQuery,
		sortQuery,
		returnData,
	)

	return r.executeQuery(context.Background(), request)
}
