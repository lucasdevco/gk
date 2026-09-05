import { defineConfig } from "@hey-api/openapi-ts";

export default defineConfig({
  input: "../api/openapi.yaml",
  output: {
    path: "src/lib/api-client",
    postProcess: ["prettier"],
  },
  plugins: ["@hey-api/client-fetch"],
});
