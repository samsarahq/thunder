import React from "react";
import { GraphQLData } from "./connection";
import { Query } from "./query";

export function graphql<
  InnerProps,
  Result extends object,
  Input extends object
>(query: string, variables: (props: InnerProps) => Input) {
  return (
    Component:
      | React.ComponentClass<InnerProps & GraphQLData<Result>>
      | React.StatelessComponent<InnerProps & GraphQLData<Result>>,
  ): React.SFC<InnerProps> => {
    return (props: InnerProps) => (
      <Query<Result, Input> query={query} variables={variables(props)}>
        {data => <Component data={data} {...props} />}
      </Query>
    );
  };
}
