import ts from "typescript";
import path from "path";
import { readdirSync } from "fs";

describe("fixtures", () => {
  readdirSync(path.join(__dirname, "fixtures")).forEach(fixture => {
    it(fixture, () => {
      const program = ts.createProgram(
        [path.join(__dirname, "fixtures", fixture)],
        {
          jsx: ts.JsxEmit.React,
          strict: true,
          noEmit: true,
          module: ts.ModuleKind.ES2015,
          target: ts.ScriptTarget.ES2015,
          allowSyntheticDefaultImports: true,
          esModuleInterop: true,
          typeRoots: ["../../node_modules/@types"],
          types: ["react", "lodash.isequal"],
          moduleResolution: ts.ModuleResolutionKind.NodeJs,
        },
      );
      const preEmitDiagnostics = ts
        .getPreEmitDiagnostics(program)
        // Get rid of file field so that diagnostics can match in snapshot testing.
        .map(err => ({ ...err, file: undefined }));

      expect(preEmitDiagnostics).toMatchSnapshot();
    });
  });
});
