import { defineConfig } from '@hey-api/openapi-ts';

export default defineConfig({
  input: './spec/swagger.json',
  output: 'packages/sdk/src',
  plugins: [
    '@hey-api/typescript',
    {
      name: '@hey-api/transformers',
      dates: true,
      bigInt: true,
    },
    {
      name: '@hey-api/sdk',
      transformer: true
    },
    '@hey-api/client-fetch',
    {
      name: '@pinia/colada',
      $hooks: {
        operations: {
          // An SSE stream returns { stream }, so it cannot be represented by
          // Pinia Colada's generated mutation contract, which expects { data }.
          isMutation: operation => operation.method === 'post'
            && operation.path === '/bots/{bot_id}/container/display/prepare'
            ? false
            : undefined,
        },
      },
    },
  ],
})
