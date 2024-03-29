package federation

import (
	"sort"

	"github.com/samsarahq/thunder/graphql"
)

/*
-----------------------------------------
Begin: Recursive Selection Set Generation
-----------------------------------------
This code is used to recursively generate a selection set for an object. The purpose is to allow for passing entire object (not scalar)
fields from team gql service -> team gql service. This can be used when the object fetch from db is too complicated/expensive and its easier
to simply pass the object rather than refetch it from keys.
Example:
Let's say we have object Devices that has been federated:
		type Devices struct
		{
			Id int | key
			Location Location | key
			Optional str
		}
		type Location struct {
			lat str | key
			lng str | key
			altitude AltitudeInput
		}
And in fetchObjectFromKeys, we have defined keys as stated above. Location is an object, so for it to be a key the
selection set needs to recursively have its fields.
This is a simplified plan for a query that queries for Id and Location on devices. The selection set is generated by the code below
		Plan {
			Type: Devices
			Service: alpha
			SelectionSet: {
				Id
				Location
					lat
					lng
			}
		}

NOTE: In this example, Location also needs to be a federated object. In other words, it needs to be defined on this server with a corresponding
fetchObjectFromKeys function
*/
func getFederatedSelectionsForObject(typ *graphql.Object, service string, selectionsByService map[string][]*graphql.Selection) *graphql.SelectionSet {
	selections := []*graphql.Selection{}

	// Sort names to maintain consistency
	names := make([]string, 0, len(typ.Fields))
	for name, _ := range typ.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		field := typ.Fields[name]
		if name == "_federation" {
			continue
		}

		serviceFound := false
		for service := range field.FederatedKey {
			if len(selectionsByService[service]) > 0 {
				serviceFound = true
				break
			}
		}

		if !serviceFound {
			continue
		}

		switch fieldType := field.Type.(type) {
		case *graphql.Object:
			ssResult := getFederatedSelectionsForObject(fieldType, service, selectionsByService)
			selections = append(selections, &graphql.Selection{
				Name:         name,
				Alias:        name,
				UnparsedArgs: map[string]interface{}{},
				SelectionSet: ssResult,
			})
		case *graphql.NonNull:
			ssResult := getFederatedSelectionsForNonNull(fieldType, name, service, selectionsByService)
			selections = append(selections, ssResult)
		case *graphql.List:
			ssResult := getFederatedSelectionsForList(fieldType, name, service, selectionsByService)
			selections = append(selections, ssResult)
		case *graphql.Scalar:
			selections = append(selections, &graphql.Selection{
				Name:         name,
				Alias:        name,
				UnparsedArgs: map[string]interface{}{},
			})
		default:
			selections = append(selections, &graphql.Selection{
				Name:         name,
				Alias:        name,
				UnparsedArgs: map[string]interface{}{},
			})
		}
	}

	return &graphql.SelectionSet{
		Selections: selections,
	}
}

func getFederatedSelectionsForList(typ *graphql.List, name string, service string, selectionsByService map[string][]*graphql.Selection) *graphql.Selection {
	var selectionResult *graphql.Selection
	switch fieldType := typ.Type.(type) {
	case *graphql.Object:
		ssResult := getFederatedSelectionsForObject(fieldType, service, selectionsByService)
		selectionResult = &graphql.Selection{
			Name:         name,
			Alias:        name,
			UnparsedArgs: map[string]interface{}{},
			SelectionSet: ssResult,
		}
	case *graphql.NonNull:
		selectionResult = getFederatedSelectionsForNonNull(fieldType, name, service, selectionsByService)
	case *graphql.List:
		selectionResult = getFederatedSelectionsForList(fieldType, name, service, selectionsByService)
	case *graphql.Scalar:
		selectionResult = &graphql.Selection{
			Name:         name,
			Alias:        name,
			UnparsedArgs: map[string]interface{}{},
		}
	default:
		selectionResult = &graphql.Selection{
			Name:         name,
			Alias:        name,
			UnparsedArgs: map[string]interface{}{},
		}
	}

	return selectionResult
}

func getFederatedSelectionsForNonNull(typ *graphql.NonNull, name string, service string, selectionsByService map[string][]*graphql.Selection) *graphql.Selection {
	var selectionResult *graphql.Selection
	switch fieldType := typ.Type.(type) {
	case *graphql.Object:
		ssResult := getFederatedSelectionsForObject(fieldType, service, selectionsByService)
		selectionResult = &graphql.Selection{
			Name:         name,
			Alias:        name,
			UnparsedArgs: map[string]interface{}{},
			SelectionSet: ssResult,
		}
	case *graphql.NonNull:
		selectionResult = getFederatedSelectionsForNonNull(fieldType, name, service, selectionsByService)
	case *graphql.List:
		selectionResult = getFederatedSelectionsForList(fieldType, name, service, selectionsByService)
	case *graphql.Scalar:
		selectionResult = &graphql.Selection{
			Name:         name,
			Alias:        name,
			UnparsedArgs: map[string]interface{}{},
		}
	default:
		selectionResult = &graphql.Selection{
			Name:         name,
			Alias:        name,
			UnparsedArgs: map[string]interface{}{},
		}
	}

	return selectionResult
}

/*
---------------------------------------
End: Recursive Selection Set Generation
---------------------------------------
*/
