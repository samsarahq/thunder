import { merge } from "../merge";

const cases: Array<{
  title: string;
  base: any;
  patch: any;
  expectation?: any;
}> = [
  {
    title: "scalar field",
    base: { name: "bob" },
    patch: { name: "dean" },
    expectation: { name: "dean" },
  },
  {
    title: "complex field",
    base: { friends: ["alice", "charlie"] },
    patch: { friends: [["eli"]] },
    expectation: { friends: ["eli"] },
  },
  {
    title: "Array no reordering",
    base: [{ name: "bob", age: 20 }, { name: "alice" }],
    patch: { "0": { name: "dean" } },
    expectation: [{ name: "dean", age: 20 }, { name: "alice" }],
  },
  {
    title: "Array with reordering",
    base: [{ name: "bob", age: 20 }, { name: "alice" }],
    patch: { $: [1, 0], "1": { age: 23 } },
    expectation: [{ name: "alice" }, { name: "bob", age: 23 }],
  },
  {
    title: "Array with a run of reordering",
    base: [
      { name: "alice" },
      { name: "bob" },
      { name: "carol" },
      { name: "dean" },
    ],
    patch: { $: [[1, 3], -1], "3": [{ name: "eli" }] },
    expectation: [
      { name: "bob" },
      { name: "carol" },
      { name: "dean" },
      { name: "eli" },
    ],
  },
  {
    title: "Map",
    base: { name: "bob", address: { state: "ca", city: "sf" }, age: 30 },
    patch: {
      name: "alice",
      address: { city: "oakland" },
      age: [],
      friends: [["bob", "charlie"]],
    },
    expectation: {
      name: "alice",
      address: { state: "ca", city: "oakland" },
      friends: ["bob", "charlie"],
    },
  },
];

describe("merge", () => {
  it("passes tests ported from go", () => {
    for (const { base, patch, expectation } of cases) {
      expect(merge(base, patch)).toEqual(expectation);
    }
  });
});
