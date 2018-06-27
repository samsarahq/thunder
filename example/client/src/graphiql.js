import React from 'react';
import GraphiQL from 'graphiql';

import { connection, mutate } from 'thunder-react';

import '../node_modules/graphiql/graphiql.css';

function graphQLFetcher({query, variables}) {
  if (query.startsWith("mutation")) {
    return mutate({
      query,
      variables,
    });
  }
  return {
    subscribe(subscriber) {
      const next = subscriber.next || subscriber;

      const subscription = connection.subscribe({
        query: query,
        variables: {},
        observer: ({state, valid, error, value}) => {
          if (valid) {
            next({data: value});
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

export function GraphiQLWithFetcher() {
  return <GraphiQL fetcher={graphQLFetcher} />;
}
