import React from "react";
import { Connection } from "./connection";

import { Consumer } from "./context";
import { Omit, Overwrite } from "./diff";

export interface MutationSpec<Result, Input> {
  query: string;
  result?: Result;
  variables?: Input;
}

interface MutationProps<Result extends object, Input extends object> {
  children: (run: (variables: Input) => Promise<Result>) => React.ReactNode;
  query: string | MutationSpec<Result, Input>;
}

type MutationPropsWithStringQuery<
  Result extends object,
  Input extends object
> = Overwrite<MutationProps<Result, Input>, { query: string }>;

export function Mutation<Result extends object, Input extends object>(
  props: MutationProps<Result, Input>,
) {
  let passedProps: Pick<
    MutationProps<Result, Input>,
    Exclude<keyof MutationProps<Result, Input>, "query">
  >;
  let query: string;

  if (typeof props.query === "string") {
    ({ query, ...passedProps } = props as MutationPropsWithStringQuery<
      Result,
      Input
    >);
  } else {
    const { query: querySpec, ...shouldPass } = props;
    passedProps = shouldPass;
    query = querySpec.query;
  }

  const child = (connection: Connection) => {
    const runMutation = (variables: Input) => {
      return connection.mutate<Input, Result>({
        query,
        variables,
      });
    };

    return props.children(runMutation);
  };

  return <Consumer>{child}</Consumer>;
}
