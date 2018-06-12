import React from "react";
import { GraphQLData } from "../connection";
import { graphql } from "../hoc";
import { QuerySpec } from "../query";

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
  return <div>{props.data.value.output}</div>;
};

const ConnectedComponentWithQueryAndVariables = graphql<
  Props,
  Result,
  Variables
>("query test {}", props => ({ y: props.input }))(PresentationalComponent);

const instanceWithQueryAndVariables = (
  <ConnectedComponentWithQueryAndVariables input="test" />
);

const ConnectedComponentWithSpec = graphql(exampleQuery, props => ({
  y: "string",
}))(PresentationalComponent);

const instanceWithSpec = <ConnectedComponentWithSpec input="test" />;
