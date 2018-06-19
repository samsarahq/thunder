import React from "react";
import { Connection } from "./connection";

import { Consumer } from "./context";
import { Omit, Overwrite } from "./diff";
import { MutationSpec } from "./spec";

type RunMutation<
  Result extends object,
  Input extends object | undefined = undefined
> = Input extends undefined
  ? (variables?: undefined) => Promise<Result>
  : (variables: Input) => Promise<Result>;

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
      return connection.mutate({
        query,
        // This isn't quite right. Not sure how to unify the union
        // type into a form that the conditional type will be okay with.
        variables: variables as Exclude<Input, undefined>,
      });
    };

    // Not sure how to unify these types. runMutation is combined
    // at the argument level, while the external type is conditional.
    return props.children(runMutation as any);
  };

  return <Consumer>{child}</Consumer>;
}
