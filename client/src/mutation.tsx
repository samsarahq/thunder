import React from "react";
import { Connection } from "./connection";

import { Consumer } from "./context";
import { Omit, Overwrite } from "./diff";

type RunMutation<
  Result extends object,
  Input extends object | undefined = undefined
> = Input extends undefined
  ? (variables?: undefined) => Promise<Result>
  : (variables: Input) => Promise<Result>;

export interface MutationSpec<
  Result extends object,
  Input extends object | undefined = undefined
> {
  query: string;
  result?: Result;
  variables?: Input;
}

interface MutationProps<
  Result extends object,
  Input extends object | undefined = undefined
> {
  children: (run: RunMutation<Result, Input>) => React.ReactNode;
  query: string | MutationSpec<Result, Input>;
}

export function Mutation<
  Result extends object,
  Input extends object | undefined = undefined
>(props: MutationProps<Result, Input>) {
  let passedProps: Pick<
    MutationProps<Result, Input>,
    Exclude<keyof MutationProps<Result, Input>, "query">
  >;
  let query: string;

  if (typeof props.query === "string") {
    query = props.query;
  } else {
    query = props.query.query;
  }

  const child = (connection: Connection) => {
    const runMutation = (variables?: Input) => {
      return connection.mutate<Exclude<Input, undefined>, Result>({
        query,
        // This isn't quite right. Maybe we should change the type of
        // mutate to make variables optional based on the query.
        variables: (variables ? variables : {}) as Exclude<Input, undefined>,
      });
    };

    // Not sure how to unify these types. runMutation is combined
    // at the argument level, while the external type is conditional.
    return props.children(runMutation as any);
  };

  return <Consumer>{child}</Consumer>;
}
