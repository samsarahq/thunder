import React from "react";
import { Mutation } from "../../mutation";
import { MutationSpec } from "../../spec";

interface Result {
  output: string;
}

interface Variables {
  y: string;
}

const exampleQuery: MutationSpec<Result, Variables> = {
  query: "mutation test {}",
};

const ComponentWithQueryAndVariables: React.SFC<{}> = () => {
  return (
    <Mutation<Result, Variables> query="test">
      {runMutation => (
        <a
          onClick={async () => {
            await runMutation({ y: "test" });
          }}
        >
          Do it
        </a>
      )}
    </Mutation>
  );
};

const instanceWithQueryAndVariables = <ComponentWithQueryAndVariables />;

const ComponentWithSpec: React.SFC<{}> = () => {
  return (
    <Mutation query={exampleQuery}>
      {runMutation => (
        <a
          onClick={async () => {
            await runMutation({ y: "test" });
          }}
        >
          Do it
        </a>
      )}
    </Mutation>
  );
};

const instanceWithSpec = <ComponentWithSpec />;
