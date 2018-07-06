import React from "react";
import { Connection, GraphQLError } from "./connection";

import { isEqual } from "lodash";

import {
  GraphQLData,
  GraphQLResult,
  Subscription,
  SubscriptionState,
} from "./connection";

import { Consumer } from "./context";
import { Omit, Overwrite } from "./diff";
import { QuerySpec } from "./spec";

interface State<QueryResult> {
  state: SubscriptionState;
  query?: string;
  variables?: object;
  value?: QueryResult;
  error?: GraphQLError;
}

interface QueryPropsBase<
  Result extends object,
  Input extends object | undefined = undefined
> {
  children: (data: GraphQLResult<Result>) => React.ReactNode;
  query: string | QuerySpec<Result, Input>;
}

type MaybeVariables<
  Input extends object | undefined = undefined
> = Input extends object
  ? {
      variables: Input;
    }
  : {
      variables?: undefined;
    };

type QueryProps<
  Result extends object,
  Input extends object | undefined = undefined
> = QueryPropsBase<Result, Input> & MaybeVariables<Input>;

interface InternalQueryProps<
  Result extends object,
  Input extends object | undefined = undefined
> {
  children: (data: GraphQLResult<Result>) => React.ReactNode;
  query: string;
  variables?: Input;
}

export function Query<
  Result extends object,
  Input extends object | undefined = undefined
>(props: QueryProps<Result, Input>) {
  let query: string;

  if (typeof props.query === "string") {
    query = props.query;
  } else {
    query = props.query.query;
  }

  return (
    <Consumer>
      {connection => (
        <GraphQLRenderer
          connection={connection}
          query={query}
          variables={props.variables}
          children={props.children}
        />
      )}
    </Consumer>
  );
}

interface ConnectionProps {
  connection: Connection;
}

interface StringQueryProps {
  query: string;
}

class GraphQLRenderer<
  Result extends object,
  Input extends object | undefined = undefined
> extends React.PureComponent<
  InternalQueryProps<Result, Input> & StringQueryProps & ConnectionProps,
  State<Result>
> {
  private subscription:
    | ReturnType<typeof Connection["prototype"]["subscribe"]>
    | undefined;

  componentWillMount() {
    const { query, variables } = this.props;
    this.subscribe({ query, variables });
  }

  componentWillReceiveProps(
    nextProps: InternalQueryProps<Result, Input> &
      StringQueryProps &
      ConnectionProps,
  ) {
    const { query, variables } = nextProps;
    if (
      isEqual(query, this.props.query) &&
      isEqual(variables, this.props.variables)
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
    } as GraphQLResult<Result>;

    return this.props.children(data);
  }

  private onData(
    data: GraphQLResult<Result>,
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
    variables: inputVariables,
  }: {
    query: string;
    variables: object | undefined;
  }) {
    const variables = inputVariables ? inputVariables : {};
    this.unsubscribe();

    this.subscription = this.props.connection.subscribe({
      query,
      variables,
      observer: data =>
        this.onData(data as GraphQLResult<Result>, query, variables),
    });

    this.onData(
      this.subscription.data() as GraphQLResult<Result>,
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
