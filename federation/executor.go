package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/samsarahq/go/oops"
	"golang.org/x/sync/errgroup"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
)

const keyField = "__key"
const federationField = "__federation"
const typeNameField = "__typeName"
const minSchemaSyncIntervalSeconds = 30

// QueryRequest is sent to federated GraphQL servers by gateway service.
type QueryRequest struct {
	Query *graphql.Query
	// Metadata is an optional custom field which can be used to send metadata such as authentication
	// along with the query.
	Metadata interface{}
}

// QueryResponse is the marshalled json reponse from federated GraphQL servers.
type QueryResponse struct {
	Result []byte
	// Metadata is an optional custom field which can be used to receive metadata such as query duration
	// along with the response.
	Metadata interface{}
}

// ExecutorClient is used to send GraphQL requests from the gateway service to federated GraphQL servers.
type ExecutorClient interface {
	Execute(ctx context.Context, request *QueryRequest) (*QueryResponse, error)
}

// Executor has a map of all the executor clients such that it can execute a
// subquery on any of the federated servers.
// The planner allows it to coordinate the subqueries being sent to the federated servers
type Executor struct {
	Executors map[string]ExecutorClient
	syncer    *Syncer
}

// Syncer checks if there is a new schema available and then updates the planner as needed
type Syncer struct {
	ticker       *time.Ticker
	schemaSyncer SchemaSyncer
	plannerMu    *sync.RWMutex
	planner      *Planner
}

func (e *Executor) getPlanner() *Planner {
	e.syncer.plannerMu.RLock()
	defer e.syncer.plannerMu.RUnlock()
	return e.syncer.planner
}

func (e *Executor) setPlanner(p *Planner) {
	e.syncer.plannerMu.Lock()
	defer e.syncer.plannerMu.Unlock()
	e.syncer.planner = p
}

func NewPlanner(types *SchemaWithFederationInfo) (*Planner, error) {
	flattener, err := newFlattener(types.Schema)
	if err != nil {
		return nil, oops.Wrapf(err, "flattening schemas error")
	}
	// The planner is aware of the merged schema and what executors
	// know about what fields
	planner := &Planner{
		schema:    types,
		flattener: flattener,
	}
	return planner, err
}

func fetchSchema(ctx context.Context, e ExecutorClient, metadata interface{}) (*QueryResponse, error) {
	query, err := graphql.Parse(introspection.IntrospectionQuery, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	return e.Execute(ctx, &QueryRequest{
		Query:    query,
		Metadata: metadata,
	})
}

type CustomExecutorArgs struct {
	SchemaSyncer              SchemaSyncer
	OptionalArgs              interface{}
	SchemaSyncIntervalSeconds func(ctx context.Context) int64
}

func NewExecutor(ctx context.Context, executors map[string]ExecutorClient, c *CustomExecutorArgs) (*Executor, error) {
	if c.SchemaSyncer == nil {
		c.SchemaSyncer = NewIntrospectionSchemaSyncer(ctx, executors, c.OptionalArgs)
	}
	if c.SchemaSyncIntervalSeconds == nil {
		c.SchemaSyncIntervalSeconds = func(ctx context.Context) int64 { return minSchemaSyncIntervalSeconds }
	}

	planner, err := c.SchemaSyncer.FetchPlanner(ctx, c.OptionalArgs)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to load schema")
	}

	schemaSyncIntervalSeconds := c.SchemaSyncIntervalSeconds(ctx)

	executor := &Executor{
		Executors: executors,
		syncer: &Syncer{
			ticker:       time.NewTicker(time.Duration(schemaSyncIntervalSeconds) * time.Second),
			schemaSyncer: c.SchemaSyncer,
			plannerMu:    &sync.RWMutex{},
			planner:      planner,
		},
	}
	go executor.poll(ctx, c.OptionalArgs)
	return executor, nil
}

func (e *Executor) poll(ctx context.Context, optionalArgs interface{}) error {
	for {
		select {
		case <-e.syncer.ticker.C:
			newPlanner, err := e.syncer.schemaSyncer.FetchPlanner(ctx, optionalArgs)
			if err == nil && newPlanner != nil {
				e.setPlanner(newPlanner)
			}
		case <-ctx.Done():
			e.syncer.ticker.Stop()
			return ctx.Err()
		}
	}
	return nil
}

func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, kind string, selectionSet *graphql.SelectionSet, optionalArgs interface{}, planner *Planner) ([]interface{}, interface{}, error) {
	// Execute query on specified service
	executorClient, ok := e.Executors[service]
	if !ok {
		return nil, nil, oops.Errorf("service not recognized")
	}

	// If it is not a root query, nest the subquery on the federation field
	// and pass the keys in to find the object that the subquery is nested on
	// Pass all federated keys for that service as arguments
	// {
	//   __federation {
	//     [ObjectName]-[Service] (keys: Keys) {
	//       subQuery
	//     }
	//   }
	// }
	isRoot := keys == nil
	if !isRoot {
		federatedName := fmt.Sprintf("%s-%s", typName, service)

		var rootObject *graphql.Object
		var ok bool
		for f, _ := range planner.schema.Fields {
			if f.Type.String() == typName {
				rootObject, ok = f.Type.(*graphql.Object)
				if !ok {
					return nil, nil, oops.Errorf("root object isn't a graphql object")
				}
			}
		}
		if rootObject == nil {
			return nil, nil, oops.Errorf("root object not found for type %s", typName)
		}

		// If it is a federated key on that service, add it to the input args
		// passed in to the federated field func as one of the federated keys
		newKeys := make([]interface{}, len(keys))

		for i, key := range keys {
			keyFields, ok := key.(map[string]interface{})
			if !ok {
				return nil, nil, oops.Errorf("key field is an incorrect type expected map[string]interface{} got %s", reflect.TypeOf(typName))
			}
			newKey := make(map[string]interface{}, len(keyFields))
			for name, keyField := range keyFields {
				if name == "__key" {
					continue
				}
				for fieldName, field := range rootObject.Fields {
					if fieldName == name {
						_, ok := field.FederatedKey[service]
						if ok {
							newKey[name] = keyField
						}
					}
				}
			}
			newKeys[i] = newKey
		}

		selectionSet = &graphql.SelectionSet{
			Selections: []*graphql.Selection{
				{
					Name:  federationField,
					Alias: federationField,
					Args:  map[string]interface{}{},
					SelectionSet: &graphql.SelectionSet{
						Selections: []*graphql.Selection{
							{
								Name:  federatedName,
								Alias: federatedName,
								UnparsedArgs: map[string]interface{}{
									"keys": newKeys,
								},
								SelectionSet: selectionSet,
							},
						},
					},
				},
			},
		}
	}

	// Execute query on specified service
	request := &QueryRequest{
		Query: &graphql.Query{
			Kind:         kind,
			SelectionSet: selectionSet,
		},
		Metadata: optionalArgs,
	}
	response, err := executorClient.Execute(ctx, request)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "execute remotely")
	}
	// Unmarshal json from results
	var res interface{}
	if err := json.Unmarshal(response.Result, &res); err != nil {
		return nil, nil, oops.Wrapf(err, "unmarshal res")
	}

	if !isRoot {
		result, ok := res.(map[string]interface{})
		if !ok {
			return nil, nil, oops.Errorf("executor res not a map[string]interface{}")
		}
		result, ok = result[federationField].(map[string]interface{})
		if !ok {
			return nil, nil, oops.Errorf("executor res not a map[string]interface{}")
		}
		federatedName := fmt.Sprintf("%s-%s", typName, service)
		r, ok := result[federatedName].([]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("root did not have a federation map, got %v", res)
		}
		return r, response.Metadata, nil

	}
	return []interface{}{res}, response.Metadata, nil
}

func (pathTargets *pathSubqueryMetadata) extractKeys(node interface{}, path []PathStep) error {
	// Extract key for every element in the slice
	if slice, ok := node.([]interface{}); ok {
		for i, elem := range slice {
			if err := pathTargets.extractKeys(elem, path); err != nil {
				return oops.Errorf("idx %d: %v", i, err)
			}
		}
		return nil
	}

	if len(path) == 0 {
		obj, ok := node.(map[string]interface{})
		if !ok {
			return fmt.Errorf("not an object: %v", obj)
		}
		key, ok := obj[federationField]
		if !ok {
			return fmt.Errorf("missing __federation: %v", obj)
		}
		// Add a pointer to the object for where the results from
		// the subquery will be added into the final result
		pathTargets.results = append(pathTargets.results, obj)
		// Keys from the "__federation" field func are passed to
		// the subquery
		pathTargets.keys = append(pathTargets.keys, key)
		return nil
	}

	obj, ok := node.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract keys nested on the object
	step := path[0]
	switch step.Kind {
	case KindField:
		next, ok := obj[step.Name]
		if !ok {
			return fmt.Errorf("does not have key %s", step.Name)
		}
		if err := pathTargets.extractKeys(next, path[1:]); err != nil {
			return fmt.Errorf("elem %s: %v", next, err)
		}
	case KindType:
		typ, ok := obj["__typename"].(string)
		if !ok {
			return fmt.Errorf("does not have string key __typename")
		}
		if typ == step.Name {
			if err := pathTargets.extractKeys(obj, path[1:]); err != nil {
				return fmt.Errorf("typ %s: %v", typ, err)
			}
		}
	default:
		return fmt.Errorf("unsupported step type name: %s kind: %v", step.Name, step.Kind)
	}

	return nil
}

func (e *Executor) execute(ctx context.Context, p *Plan, keys []interface{}, optionalArgs interface{}, planner *Planner) ([]interface{}, []interface{}, error) {
	var res []interface{}
	optionalRespMetadata := make([]interface{}, 0)
	// var optionalResponseArg interface{}
	// Executes that part of the plan (the subquery) on one of the federated gqlservers
	if p.Service != gatewayCoordinatorServiceName {
		var err error
		var optionalRespQueryMetaData interface{}
		res, optionalRespQueryMetaData, err = e.runOnService(ctx, p.Service, p.Type, keys, p.Kind, p.SelectionSet, optionalArgs, planner)
		if err != nil {
			return nil, nil, oops.Wrapf(err, "run on service")
		}
		optionalRespMetadata = append(optionalRespMetadata, optionalRespQueryMetaData)
	} else {
		res = []interface{}{
			map[string]interface{}{},
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	// resMu protects the results (res) as we stitch the results together from seperate goroutines
	// executing in different parts of the plan on different services
	var resMu sync.Mutex

	// For every nested query in the plan, execute it on the specified service and stitch
	// the results into a response
	for _, currentSubPlan := range p.After {
		subPlan := currentSubPlan
		var subPlanMetaData pathSubqueryMetadata
		if p.Service == gatewayCoordinatorServiceName {
			subPlanMetaData.keys = nil // On the root query there are no specified keys
			// On the root query, there will only be one result since
			// it is on either the "query" or "mutation object"
			subPlanMetaData.results = []map[string]interface{}{
				res[0].(map[string]interface{}),
			}
			subPlanMetaData.optionalResponseMetatda = nil
		} else {
			if err := subPlanMetaData.extractKeys(res, subPlan.Path); err != nil {
				return nil, nil, fmt.Errorf("failed to extract keys %v: %v", subPlan.Path, err)
			}
		}

		g.Go(func() error {
			// Execute the subquery on the specified service
			executionResults, subQueryRespMetadata, err := e.execute(ctx, subPlan, subPlanMetaData.keys, optionalArgs, planner)
			if err != nil {
				return oops.Wrapf(err, "executing sub plan: %v", err)
			}
			optionalRespMetadata = append(optionalRespMetadata, subQueryRespMetadata...)

			if len(executionResults) != len(subPlanMetaData.results) {
				return fmt.Errorf("got %d results for %d targets", len(executionResults), len(subPlanMetaData.results))
			}

			// Acquire mutex lock before modifying results
			resMu.Lock()
			defer resMu.Unlock()
			for i, result := range subPlanMetaData.results {
				executionResult, ok := executionResults[i].(map[string]interface{})
				if !ok {
					return fmt.Errorf("result is not an object: %v", executionResult)
				}

				for k, v := range executionResult {
					if _, ok := result[k]; !ok {
						result[k] = v
					} else {
						if k != keyField || v != result[k] {
							return oops.Errorf("key already exists in results: %v", k)
						}
					}
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	return res, optionalRespMetadata, nil
}

func deleteKey(v interface{}, k string) {
	switch v := v.(type) {
	case []interface{}:
		for _, e := range v {
			deleteKey(e, k)
		}
	case map[string]interface{}:
		delete(v, k)
		for _, e := range v {
			deleteKey(e, k)
		}
	}
}

// Metadata for a subquery
type pathSubqueryMetadata struct {
	keys                    []interface{}            // Federated Keys passed into subquery
	results                 []map[string]interface{} // Results from subquery
	optionalResponseMetatda []interface{}
}

func (e *Executor) Execute(ctx context.Context, query *graphql.Query, optionalArgs interface{}) (interface{}, []interface{}, error) {
	planner := e.getPlanner()
	plan, err := planner.planRoot(query)
	if err != nil {
		return nil, nil, err
	}

	r, responseMetadata, err := e.execute(ctx, plan, nil, optionalArgs, planner)
	if err != nil {
		return nil, nil, err
	}

	if len(r) != 1 {
		return nil, nil, oops.Errorf("Multiple results, expected one %v", r)
	}
	// The interface for results assumes we always get back a list of objects
	// On the root query, we know there is only one object (a query or mutation)
	// So we expect only one item in this list
	res := r[0]
	deleteKey(res, federationField)
	return res, responseMetadata, nil
}
