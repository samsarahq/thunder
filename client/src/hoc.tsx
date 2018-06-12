import React from "react";
import { GraphQLData } from "./connection";
import { Query, QuerySpec } from "./query";
import { Omit } from "./diff";

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object,
  Input extends object
>(
  query: string,
  variables: (props: Omit<InnerProps, "data">) => Input,
): <ChildProps extends InnerProps>(
  Component: React.ComponentType<ChildProps>,
) => React.SFC<Omit<ChildProps, "data">>;

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object,
  Input extends object
>(
  query: QuerySpec<Result, Input>,
  variables: (props: Omit<InnerProps, "data">) => Input,
): <ChildProps extends InnerProps>(
  Component: React.ComponentType<ChildProps>,
) => React.SFC<Omit<ChildProps, "data">>;

export function graphql<
  InnerProps extends GraphQLData<Result>,
  Result extends object,
  Input extends object
>(
  query: string | QuerySpec<Result, Input>,
  variables: (
    props: Omit<InnerProps, "data">,
  ) => typeof query extends string
    ? Input
    : Exclude<Exclude<typeof query, string>["variables"], undefined>,
) {
  type ResultType = typeof query extends string
    ? Result
    : Exclude<Exclude<typeof query, string>["result"], undefined>;
  type InputType = typeof query extends string
    ? Result
    : Exclude<Exclude<typeof query, string>["variables"], undefined>;

  return <ChildProps extends InnerProps>(
    Component: React.ComponentType<ChildProps>,
  ): React.SFC<Omit<ChildProps, "data">> => {
    return props => (
      <Query<ResultType, InputType>
        query={query as string | QuerySpec<ResultType, InputType>}
        variables={variables(props)}
      >
        {data => <Component data={data} {...props} />}
      </Query>
    );
  };
}
