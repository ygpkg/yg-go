package esquery

import (
	"testing"

	jsoniter "github.com/json-iterator/go"
)

func TestBuilder(t *testing.T) {
	// 对应原生 DSL：
	// GET /pt_patent_sup/_search
	// {
	//	"_source": ["document_number", "field_name", "content"],
	//	"query": {
	//		"bool": {
	//			"must": [{
	//				"terms": {
	//					"document_number": ["CN212195649U"]
	//				}
	//			}, {
	//				"match": {
	//					"content": "汽车"
	//				}
	//			}]
	//		}
	//	},
	//	"highlight": {
	//		"pre_tags": ["\u003cem\u003e"],
	//		"post_tags": ["\u003c/em\u003e"],
	//		"fields": {
	//			"content": {}
	//		},
	//		"fragment_size": 200,
	//		"number_of_fragments": 5
	//	},
	//	"sort": {
	//		"create_time": {
	//			"order": "desc"
	//		}
	//	},
	//	"size": 10
	// }
	boolQuery := BuildMap("must", []Map{
		BuildMap("terms", BuildMap("document_number", []string{"CN212195649U"})),
		BuildMap("match", BuildMap("content", "汽车")),
	})

	query := NewBuilder().
		SetSource([]string{"document_number", "field_name", "content"}).
		SetQuery(BuildMap("bool", boolQuery)).
		SetHighlight(BuildHighlightField([]string{"content"}, WithFragmentSize(200))).
		SetSort(BuildSortField("create_time", "desc")).
		SetSize(10).
		Build()
	searchBody, _ := jsoniter.MarshalToString(query)
	t.Log(searchBody)

}
