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

const PresentationalComponent = (props: Props) => null;

const ConnectedComponentWithQueryAndVariables = graphql<
  Props,
  Result,
  Variables
>(PresentationalComponent, "query test {}");

const ConnectedComponentWithSpec = graphql(
  PresentationalComponent,
  exampleQuery,
);

const exampleQueryWithoutVariables: QuerySpec<Result> = {
  query: "query test {}",
};

const withQVEmptyObject = graphql<Props, Result>(
  PresentationalComponent,
  "query test {}",
  () => ({}),
);

const withQSEmptyObject = graphql(
  PresentationalComponent,
  exampleQueryWithoutVariables,
  () => ({ s: 2 }),
);
