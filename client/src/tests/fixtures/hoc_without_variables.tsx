import React from "react";
import { GraphQLData } from "../../connection";
import { graphql } from "../../hoc";
import { QuerySpec } from "../../spec";

interface Result {
  output: string;
}

const exampleQuery: QuerySpec<Result> = {
  query: "query test {}",
};

interface Props extends GraphQLData<Result> {
  input: string;
}

const PresentationalComponent = (props: Props) => {
  return (
    <div>
      {props.data.state === "subscribed" ? props.data.value.output : null}
    </div>
  );
};

const ConnectedComponentWithQueryAndVariables = graphql<Props, Result>(
  PresentationalComponent,
  "query test {}",
);

const instanceWithQueryAndVariables = (
  <ConnectedComponentWithQueryAndVariables input="test" />
);

const ConnectedComponentWithSpec = graphql(
  PresentationalComponent,
  exampleQuery,
);

const instanceWithSpec = <ConnectedComponentWithSpec input="test" />;
