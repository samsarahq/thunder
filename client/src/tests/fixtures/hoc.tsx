import React from "react";
import { GraphQLData } from "../../connection";
import { graphql } from "../../hoc";
import { QuerySpec } from "../../spec";

interface Result {
  output: string;
}

interface Variables {
  y: string;
}

const exampleQuery: QuerySpec<Result, Variables> = {
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

const ConnectedComponentWithQueryAndVariables = graphql<
  Props,
  Result,
  Variables
>(PresentationalComponent, "query test {}", props => ({ y: props.input }));

const instanceWithQueryAndVariables = (
  <ConnectedComponentWithQueryAndVariables input="test" />
);

const ConnectedComponentWithSpec = graphql(
  PresentationalComponent,
  exampleQuery,
  props => ({
    y: props.input,
  }),
);

const instanceWithSpec = <ConnectedComponentWithSpec input="test" />;
