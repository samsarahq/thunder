import React from 'react';
import { buildSchema } from 'graphql';
import GraphiQL from 'graphiql';

import { connection } from './store';

import '../node_modules/graphiql/graphiql.css';

const graphQLSchema = buildSchema(`
  type Message {
    id: Int,
    text: String
  }

  type Query {
    messages: [Message]
  }
`);

function graphQLFetcher({query, variables}) {
  return {
    subscribe({next, error, complete}) {
      const subscription = connection.subscribe({
        query: query,
        variables: {},
        observer: ({state, valid, error, value}) => {
          if (valid) {
            next(value);
          } else {
            next({state, error});
          }
        }
      });

      return {
        unsubscribe() {
          return subscription.close();
        }
      };
    }
  };
}

export function GraphiQLWithFetcherAndSchema() {
  return <GraphiQL fetcher={graphQLFetcher} schema={graphQLSchema} />;
}
