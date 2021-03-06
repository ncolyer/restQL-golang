package eval

import (
	"fmt"
	"regexp"

	"github.com/b2wdigital/restQL-golang/v4/internal/domain"
	"github.com/b2wdigital/restQL-golang/v4/pkg/restql"
	"github.com/pkg/errors"
)

// ApplyFilters returns a version of the already resolved Resources
// only with the fields defined by the `only` clause.
func ApplyFilters(log restql.Logger, query domain.Query, resources domain.Resources) (domain.Resources, error) {
	result := make(domain.Resources)

	for _, stmt := range query.Statements {
		resourceID := domain.NewResourceID(stmt)
		dr := resources[resourceID]

		filtered, err := applyOnlyFilters(stmt.Only, dr)
		if err != nil {
			log.Error("failed to apply filter on statement", err, "statement", fmt.Sprintf("%+#v", stmt), "done-resource", fmt.Sprintf("%+#v", dr))
			return nil, err
		}

		result[resourceID] = filtered
	}

	return result, nil
}

func applyOnlyFilters(filters []interface{}, resourceResult interface{}) (interface{}, error) {
	if len(filters) == 0 {
		return resourceResult, nil
	}

	switch resourceResult := resourceResult.(type) {
	case restql.DoneResource:
		body := resourceResult.ResponseBody.Unmarshal()
		result, err := extractWithFilters(buildFilterTree(filters), body)
		if err != nil {
			return nil, err
		}
		resourceResult.ResponseBody.SetValue(result)

		return resourceResult, nil
	case restql.DoneResources:
		list := make(restql.DoneResources, len(resourceResult))
		for i, r := range resourceResult {
			list[i], _ = applyOnlyFilters(filters, r)
		}
		return list, nil
	default:
		return resourceResult, errors.Errorf("resource result has unknown type %T with value: %v", resourceResult, resourceResult)
	}
}

func extractWithFilters(filters map[string]interface{}, resourceResult interface{}) (interface{}, error) {
	filters, hasSelectAll := extractSelectAllFilter(filters)

	switch resourceResult := resourceResult.(type) {
	case map[string]interface{}:
		var node map[string]interface{}
		if hasSelectAll {
			node = resourceResult
		} else {
			node = make(map[string]interface{})
		}

		for key, subFilter := range filters {
			value, found := resourceResult[key]
			if !found {
				continue
			}

			if matchFilter, ok := subFilter.(domain.Match); ok {
				err := applyMatchFilter(matchFilter, key, value, node)
				if err != nil {
					return nil, err
				}
			} else if subFilter == nil {
				node[key] = value
			} else {
				subFilter, _ := subFilter.(map[string]interface{})
				f, err := extractWithFilters(subFilter, value)
				if err != nil {
					return nil, err
				}
				node[key] = f
			}

		}

		return node, nil
	case []interface{}:
		var node []interface{}
		if hasSelectAll {
			node = resourceResult
		} else {
			node = make([]interface{}, len(resourceResult))
		}

		for i, r := range resourceResult {
			f, err := extractWithFilters(filters, r)
			if err != nil {
				return nil, err
			}
			node[i] = f
		}

		return node, nil
	default:
		return resourceResult, nil
	}
}

func extractSelectAllFilter(filters map[string]interface{}) (map[string]interface{}, bool) {
	m := make(map[string]interface{})
	has := false

	for k, v := range filters {
		if k != "*" {
			m[k] = v
		} else {
			has = true
		}
	}

	return m, has
}

func applyMatchFilter(filter domain.Match, key string, value interface{}, node map[string]interface{}) error {
	matchRegex, err := parseMatchArg(filter.Arg)
	if err != nil {
		return err
	}

	switch value := value.(type) {
	case []interface{}:
		var list []interface{}

		for _, v := range value {
			strVal := fmt.Sprintf("%v", v)
			match := matchRegex.MatchString(strVal)
			if match {
				list = append(list, v)
			}
		}

		if len(list) > 0 {
			node[key] = list
		}

		return nil
	default:
		strVal := fmt.Sprintf("%v", value)
		match := matchRegex.MatchString(strVal)

		if match {
			node[key] = value
		} else {
			delete(node, key)
		}

		return nil
	}
}

func parseMatchArg(arg interface{}) (*regexp.Regexp, error) {
	switch arg := arg.(type) {
	case *regexp.Regexp:
		return arg, nil
	case string:
		return regexp.Compile(arg)
	default:
		return nil, errors.New("failed to parse match argument : unknown match argument type")
	}
}

func buildFilterTree(filters []interface{}) map[string]interface{} {
	tree := make(map[string]interface{})

	for _, f := range filters {
		path := parsePath(f)
		buildPathInTree(path, tree)
	}

	return tree
}

func buildPathInTree(path []interface{}, tree map[string]interface{}) {
	if len(path) == 0 {
		return
	}

	var field string
	var leaf interface{}

	switch f := path[0].(type) {
	case string:
		field = f
		leaf = nil
	case domain.Match:
		fields, ok := f.Target().([]string)
		if !ok {
			return
		}

		field = fields[0]
		leaf = f
	}

	if len(path) == 1 {
		tree[field] = leaf
		return
	}

	if subNode, found := tree[field]; found {
		subNode, ok := subNode.(map[string]interface{})
		if !ok {
			subNode = make(map[string]interface{})
			tree[field] = subNode
		}

		buildPathInTree(path[1:], subNode)
	} else {
		subNode := make(map[string]interface{})
		tree[field] = subNode
		buildPathInTree(path[1:], subNode)
	}

}

func parsePath(s interface{}) []interface{} {
	switch s := s.(type) {
	case []string:
		items := s

		result := make([]interface{}, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result
	case domain.Match:
		items, ok := s.Target().([]string)
		if !ok {
			return nil
		}

		result := make([]interface{}, len(items))
		for i, item := range items {
			if i == len(items)-1 {
				result[i] = domain.Match{Value: []string{item}, Arg: s.Arg}
			} else {
				result[i] = item
			}
		}
		return result
	default:
		return nil
	}
}

// ApplyHidden returns a version of the already resolved Resources
// removing the statement results with the `hidden` clause.
func ApplyHidden(query domain.Query, resources domain.Resources) domain.Resources {
	result := make(domain.Resources)

	for _, stmt := range query.Statements {
		if stmt.Hidden {
			continue
		}
		resourceID := domain.NewResourceID(stmt)
		dr := resources[resourceID]

		result[resourceID] = dr
	}

	return result
}
