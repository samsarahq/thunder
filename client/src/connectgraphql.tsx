import React, { createContext } from "react";
import { Connection, GraphQLError } from "./connection";
import { newMockWebSocket } from "./mocks/websocket";

import { isEqual } from "lodash";

import {
  GraphQLData,
  GraphQLResult,
  Subscription,
  SubscriptionState,
} from "./connection";

import { Consumer } from "./context";

interface State<QueryResult> {
  state: SubscriptionState;
  query?: string;
  variables?: object;
  value?: QueryResult;
  error?: GraphQLError;
}

interface QueryProps<
  QueryResult extends object,
  QueryInputVariables extends object
> {
  children: (data: GraphQLResult<QueryResult>) => React.ReactNode;
  query: string;
  variables: QueryInputVariables;
}

export function Query<Result extends object, Input extends object>(
  props: QueryProps<Result, Input>,
) {
  return (
    <Consumer>
      {connection => <GraphQLRenderer connection={connection} {...props} />}
    </Consumer>
  );
}

class GraphQLRenderer<
  QueryResult extends object,
  QueryInputVariables extends object
> extends React.PureComponent<
  QueryProps<QueryResult, QueryInputVariables> & { connection: Connection },
  State<QueryResult>
> {
  private subscription:
    | ReturnType<typeof Connection["prototype"]["subscribe"]>
    | undefined;

  componentWillMount() {
    const { query, variables } = this.props;
    this.subscribe({ query, variables });
  }

  componentWillReceiveProps(
    nextProps: QueryProps<QueryResult, QueryInputVariables>,
  ) {
    const { query, variables } = nextProps;
    if (
      isEqual(query, this.state.query) &&
      isEqual(variables, this.state.variables)
    ) {
      return;
    }
    this.subscribe({ query, variables });
  }

  componentWillUnmount() {
    this.unsubscribe();
  }

  render() {
    const { query, variables } = this.props;

    // If the current state is valid (subscribed/cached), we can render the data.
    const hasValidValue =
      this.state.state === "subscribed" || this.state.state === "cached";

    // If we are loading a new query and the previous query is the same (but with different query variables),
    // show the last value if it exists.
    const hasPreviousValue =
      this.state.state === "loading" && isEqual(query, this.state.query);

    // If we are rendering an error, show the last value if it exists are the query
    // and its variables are exactly the same. (i.e. a refresh of the data failed).
    const hasValueFromBeforeError =
      this.state.state === "error" &&
      isEqual(query, this.state.query) &&
      isEqual(variables, this.state.variables);

    const shouldRenderValue =
      hasValidValue || hasPreviousValue || hasValueFromBeforeError;

    const data = {
      value: shouldRenderValue ? this.state.value : undefined,
      state: this.state.state,
      error: this.state.error,
    } as GraphQLResult<QueryResult>;

    return this.props.children(data);
  }

  private onData(
    data: GraphQLResult<QueryResult>,
    query: string,
    variables: object,
  ) {
    let partialState = null;
    if (data.state === "subscribed" || data.state === "cached") {
      partialState = {
        value: data.value,
        query,
        variables,
      };
    }

    this.setState({
      state: data.state,
      error: data.error,
      ...partialState,
    });
  }

  private subscribe({
    query,
    variables,
  }: {
    query: string;
    variables: object;
  }) {
    this.unsubscribe();

    this.subscription = this.props.connection.subscribe({
      query,
      variables,
      observer: data =>
        this.onData(data as GraphQLResult<QueryResult>, query, variables),
    });

    this.onData(
      this.subscription.data() as GraphQLResult<QueryResult>,
      query,
      variables,
    );
  }

  private unsubscribe() {
    if (this.subscription) {
      this.subscription.close();
      this.subscription = undefined;
    }
  }
}
