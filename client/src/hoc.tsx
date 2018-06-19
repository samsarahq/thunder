import React from "react";
import { GraphQLData } from "./connection";
import { Query } from "./query";
import { QuerySpec } from "./spec";
import { Omit } from "./diff";

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object
>(
  Component: React.ComponentType<InnerProps>,
  query: string,
): React.SFC<Omit<InnerProps, "data">>;

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object
>(
  Component: React.ComponentType<InnerProps>,
  query: QuerySpec<Result>,
): React.SFC<Omit<InnerProps, "data">>;

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object,
  Input extends object
>(
  Component: React.ComponentType<InnerProps>,
  query: string,
  variables: (props: Omit<InnerProps, "data">) => Input,
): React.SFC<Omit<InnerProps, "data">>;

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object,
  Input extends object
>(
  Component: React.ComponentType<InnerProps>,
  query: QuerySpec<Result, Input>,
  variables: (props: Omit<InnerProps, "data">) => Input,
): React.SFC<Omit<InnerProps, "data">>;

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object,
  Input extends object | undefined = undefined
>(
  Component: React.ComponentType<InnerProps>,
  query: string | QuerySpec<Result, Input>,
  variables?:
    | ((
        props: Omit<InnerProps, "data">,
      ) => typeof query extends string
        ? Input
        : Exclude<Exclude<typeof query, string>["variables"], undefined>)
    | undefined,
): React.SFC<Omit<InnerProps, "data">> {
  type ResultType = typeof query extends string
    ? Result
    : Exclude<Exclude<typeof query, string>["result"], undefined>;
  type InputType = typeof query extends string
    ? Result
    : Exclude<Exclude<typeof query, string>["variables"], undefined>;

  return props => (
    <Query<ResultType, InputType>
      query={query as string | QuerySpec<ResultType, InputType>}
      variables={variables ? variables(props) : {}}
    >
      {data => <Component data={data} {...props} />}
    </Query>
  );
}
