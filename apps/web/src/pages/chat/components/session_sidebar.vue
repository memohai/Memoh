<template>
  <section :class="mountNode.id">
    <Teleport :to="mountNode.rightDefault">
      <SidebarProvider :open="sidebarOpen">
        <SidebarInset>
          <Sidebar
            class="absolute! bg-red-500 overflow-hidden"
            side="right"
            collapsible="icon"
          >
            <SidebarHeader class="h-12 flex flex-rows justify-center">
              <section class="flex items-center gap-2">                
                <img
                  src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTgiIGhlaWdodD0iMTgiIHZpZXdCb3g9IjAgMCAxOCAxOCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTE2LjUgOUMxNi41IDYuMTg3NyAxNi41IDQuNzgxNTUgMTUuNzgzOCAzLjc5NTgxQzE1LjU1MjUgMy40Nzc0NSAxNS4yNzI2IDMuMTk3NDggMTQuOTU0MiAyLjk2NjE5QzEzLjk2ODQgMi4yNSAxMi41NjIzIDIuMjUgOS43NSAyLjI1SDguMjVDNS40Mzc3IDIuMjUgNC4wMzE1NSAyLjI1IDMuMDQ1ODEgMi45NjYxOUMyLjcyNzQ1IDMuMTk3NDggMi40NDc0OCAzLjQ3NzQ1IDIuMjE2MTkgMy43OTU4MUMxLjUgNC43ODE1NSAxLjUgNi4xODc3IDEuNSA5QzEuNSAxMS44MTIzIDEuNSAxMy4yMTg0IDIuMjE2MTkgMTQuMjA0MkMyLjQ0NzQ4IDE0LjUyMjUgMi43Mjc0NSAxNC44MDI1IDMuMDQ1ODEgMTUuMDMzOEM0LjAzMTU1IDE1Ljc1IDUuNDM3NyAxNS43NSA4LjI1IDE1Ljc1SDkuNzVDMTIuNTYyMyAxNS43NSAxMy45Njg0IDE1Ljc1IDE0Ljk1NDIgMTUuMDMzOEMxNS4yNzI2IDE0LjgwMjUgMTUuNTUyNSAxNC41MjI1IDE1Ljc4MzggMTQuMjA0MkMxNi41IDEzLjIxODQgMTYuNSAxMS44MTIzIDE2LjUgOVoiIHN0cm9rZT0iIzBBMEEwQSIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPHBhdGggZD0iTTEwLjg3NSAyLjYyNUwxMC44NzUgMTUuMzc1IiBzdHJva2U9IiMwQTBBMEEiIHN0cm9rZS13aWR0aD0iMS41IiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0xNC4yNSA1LjI1SDEzLjEyNSIgc3Ryb2tlPSIjMEEwQTBBIiBzdHJva2Utd2lkdGg9IjEuNSIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0xNC4yNSA4LjI1SDEzLjEyNSIgc3Ryb2tlPSIjMEEwQTBBIiBzdHJva2Utd2lkdGg9IjEuNSIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik02IDcuNUw2LjkxOTkxIDguMjkyODlDNy4zMDY2MyA4LjYyNjIzIDcuNSA4Ljc5Mjg5IDcuNSA5QzcuNSA5LjIwNzExIDcuMzA2NjQgOS4zNzM3NyA2LjkxOTkxIDkuNzA3MTFMNiAxMC41IiBzdHJva2U9IiMwQTBBMEEiIHN0cm9rZS13aWR0aD0iMS41IiBzdHJva2UtbGluZWNhcD0icm91bmQiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPC9zdmc+Cg=="
                  alt="折叠"
                  class="group-data-[collapsible=icon]:m-auto"
                  @click="sidebarOpen = !sidebarOpen"
                >
                <h3 class="font-semibold text-sm group-data-[collapsible=icon]:hidden">
                  Session 源数据
                </h3>
              </section>
            </SidebarHeader>
            <Separator />
            <section
              class="not-group-data-[collapsible=icon]:hidden flex justify-center"
              style="writing-mode:sideways-rl"
            >
              <span class="flex-none w-[1em] mr-2.5 mt-4 text-muted-foreground text-sm">
                Session 源数据
              </span>
            </section>
            <SidebarContent class="group-data-[collapsible=icon]:invisible">
              <SidebarGroup>
                <SidebarGroupContent>
                  <SidebarMenu>
                    <form
                      class="**:[label]:text-xs **:[label]:uppercase **:[label]:text-muted-foreground **:[label]:tracking-wide "
                    >
                      <section class="flex flex-col gap-6">
                        <FormField
                          v-slot="{ componentField }"
                          name="username"
                        >
                          <FormItem>
                            <FormLabel>
                              Bot
                            </FormLabel>
                            <FormControl>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <img
                                    src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTQiIGhlaWdodD0iMTQiIHZpZXdCb3g9IjAgMCAxNCAxNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTEuNzUgN0M0LjM3NSA3IDcgNC4zNzUgNyAxLjc1QzcgNC4zNzUgOS42MjUgNyAxMi4yNSA3QzkuNjI1IDcgNyA5LjYyNSA3IDEyLjI1QzcgOS42MjUgNC4zNzUgNyAxLjc1IDdaIiBzdHJva2U9IiM3MzczNzMiIHN0cm9rZS13aWR0aD0iMS4xNjY2NyIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCIvPgo8cGF0aCBkPSJNMS4xNjY1IDExLjM3NUMxLjY1MjYyIDExLjM3NSAyLjYyNDg0IDEwLjQwMjggMi42MjQ4NCA5LjkxNjY2QzIuNjI0ODQgMTAuNDAyOCAzLjU5NzA2IDExLjM3NSA0LjA4MzE3IDExLjM3NUMzLjU5NzA2IDExLjM3NSAyLjYyNDg0IDEyLjM0NzIgMi42MjQ4NCAxMi44MzMzQzIuNjI0ODQgMTIuMzQ3MiAxLjY1MjYyIDExLjM3NSAxLjE2NjUgMTEuMzc1WiIgc3Ryb2tlPSIjNzM3MzczIiBzdHJva2Utd2lkdGg9IjEuMTY2NjciIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPHBhdGggZD0iTTkuMzMzMDEgMi45MTY2NkM5LjkxNjM0IDIuOTE2NjYgMTEuMDgzIDEuNzUgMTEuMDgzIDEuMTY2NjZDMTEuMDgzIDEuNzUgMTIuMjQ5NyAyLjkxNjY2IDEyLjgzMyAyLjkxNjY2QzEyLjI0OTcgMi45MTY2NiAxMS4wODMgNC4wODMzMyAxMS4wODMgNC42NjY2NkMxMS4wODMgNC4wODMzMyA5LjkxNjM0IDIuOTE2NjYgOS4zMzMwMSAyLjkxNjY2WiIgc3Ryb2tlPSIjNzM3MzczIiBzdHJva2Utd2lkdGg9IjEuMTY2NjciIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPC9zdmc+Cg=="
                                    alt="Bot"
                                  >
                                </InputGroupAddon>
                              </InputGroup>
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        </FormField>
                        <FormField
                          v-slot="{ componentField }"
                          name="username"
                        >
                          <FormItem>
                            <FormLabel>
                              Top Skills
                            </FormLabel>
                            <FormControl>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <FontAwesomeIcon
                                    :icon="['fas', 'circle']"
                                    class="w-1.5!"
                                  />
                                </InputGroupAddon>
                              </InputGroup>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <FontAwesomeIcon
                                    :icon="['fas', 'circle']"
                                    class="w-1.5!"
                                  />
                                </InputGroupAddon>
                              </InputGroup>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <FontAwesomeIcon
                                    :icon="['fas', 'circle']"
                                    class="w-1.5!"
                                  />
                                </InputGroupAddon>
                              </InputGroup>
                            </FormControl>
                            <p class="text-xs text-muted-foreground mt-4">
                              <img
                                src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTQiIGhlaWdodD0iMTQiIHZpZXdCb3g9IjAgMCAxNCAxNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTcuNTgzMzMgMTEuMzc1SDdDNS4zNTAwOCAxMS4zNzUgNC41MjUxMyAxMS4zNzUgNC4wMTI1NiAxMC44NjI0QzMuNSAxMC4zNDk4IDMuNSA5LjUyNDg5IDMuNSA3Ljg3NDk5VjYuNzA4MzJNMy41IDYuNzA4MzJWNC42NjY2Nk0zLjUgNi43MDgzMkg2LjcwODMzIiBzdHJva2U9IiM3MzczNzMiIHN0cm9rZS13aWR0aD0iMS4xNjY2NyIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik03LjU4MzUgMTEuMzc1QzcuNTgzNSAxMC42ODc1IDcuNTgzNSAxMC4zNDM4IDcuNzg4NTQgMTAuMTMwMkM3Ljk5MzUyIDkuOTE2NjYgOC4zMjM1MSA5LjkxNjY2IDguOTgzNSA5LjkxNjY2SDkuNjgzNUMxMC4zNDM1IDkuOTE2NjYgMTAuNjczNSA5LjkxNjY2IDEwLjg3ODUgMTAuMTMwMkMxMS4wODM1IDEwLjM0MzggMTEuMDgzNSAxMC42ODc1IDExLjA4MzUgMTEuMzc1QzExLjA4MzUgMTIuMDYyNCAxMS4wODM1IDEyLjQwNjIgMTAuODc4NSAxMi42MTk4QzEwLjY3MzUgMTIuODMzMyAxMC4zNDM1IDEyLjgzMzMgOS42ODM1IDEyLjgzMzNIOC45ODM1QzguMzIzNTEgMTIuODMzMyA3Ljk5MzUyIDEyLjgzMzMgNy43ODg1NCAxMi42MTk4QzcuNTgzNSAxMi40MDYyIDcuNTgzNSAxMi4wNjI0IDcuNTgzNSAxMS4zNzVaIiBzdHJva2U9IiM3MzczNzMiIHN0cm9rZS13aWR0aD0iMS4xNjY2NyIvPgo8cGF0aCBkPSJNMi40OTk4MyAxLjE2NjY2SDQuNDk5ODRDNS43MDk5NyAxLjE2NjY2IDUuODMzMTcgMS44MTQxMiA1LjgzMzE3IDIuOTE2NjZDNS44MzMxNyA0LjAxOTIgNS43MDk5NyA0LjY2NjY2IDQuNDk5ODQgNC42NjY2NkgyLjQ5OTgzQzEuMjg5NyA0LjY2NjY2IDEuMTY2NSA0LjAxOTIgMS4xNjY1IDIuOTE2NjZDMS4xNjY1IDEuODE0MTIgMS4yODk3IDEuMTY2NjYgMi40OTk4MyAxLjE2NjY2WiIgc3Ryb2tlPSIjNzM3MzczIiBzdHJva2Utd2lkdGg9IjEuMTY2NjciLz4KPHBhdGggZD0iTTEyLjI1MzQgMy4xNDA5TDEyLjYwODIgMy40OTQyOEMxMi45MDg0IDMuNzkzMjcgMTIuOTA4NCA0LjI3ODAyIDEyLjYwODIgNC41NzdMMTAuNzQ5MyA2LjQ2MzI4QzEwLjYwMzEgNi42MDg5MiAxMC40MTYxIDYuNzA3MSAxMC4yMTI4IDYuNzQ0OTFMOS4wNjA3NSA2Ljk5NDAxQzguODc4ODUgNy4wMzMzNCA4LjcxNjg4IDYuODcyNDggOC43NTU4NCA2LjY5MTJMOS4wMDEwNiA1LjU1MDI5QzkuMDM5MDMgNS4zNDc4MiA5LjEzNzYgNS4xNjE1MiA5LjI4MzgzIDUuMDE1ODhMMTEuMTY2MyAzLjE0MDg5QzExLjQ2NjUgMi44NDE5MSAxMS45NTMyIDIuODQxOTEgMTIuMjUzNCAzLjE0MDlaIiBzdHJva2U9IiM3MzczNzMiIHN0cm9rZS13aWR0aD0iMS4xNjY2NyIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+Cjwvc3ZnPgo="
                                alt="Skills"
                                class="inline-block"
                              >
                         
                              管理全部 Skills(5)
                            </p>
                          </FormItem>
                        </FormField>

                        <section>
                          <h5 class="text-xs text-muted-foreground mb-2">
                            用量统计
                          </h5>
                          <Card class="p-4!">
                            <CardContent
                              class="p-0 flex flex-col **:[section]:text-xs gap-3 text-muted-foreground *:flex *:justify-between **:[img]:inline-block **:[img]:mr-1"
                            >
                              <section>
                                <h5>
                                  <img
                                    src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTEiIGhlaWdodD0iMTEiIHZpZXdCb3g9IjAgMCAxMSAxMSIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPGcgY2xpcC1wYXRoPSJ1cmwoI2NsaXAwXzk4XzgwMSkiPgo8cGF0aCBkPSJNMTAuMDgzMiA1LjQ5OTk5SDguOTQ2NUM4Ljc0NjE5IDUuNDk5NTYgOC41NTEyNiA1LjU2NDc1IDguMzkxNTIgNS42ODU2QzguMjMxNzcgNS44MDY0NCA4LjExNiA1Ljk3NjI5IDguMDYxOTIgNi4xNjkxNkw2Ljk4NDg0IDEwLjAwMDhDNi45Nzc5IDEwLjAyNDcgNi45NjM0MiAxMC4wNDU2IDYuOTQzNTkgMTAuMDYwNEM2LjkyMzc1IDEwLjA3NTMgNi44OTk2MyAxMC4wODMzIDYuODc0ODQgMTAuMDgzM0M2Ljg1MDA0IDEwLjA4MzMgNi44MjU5MiAxMC4wNzUzIDYuODA2MDkgMTAuMDYwNEM2Ljc4NjI1IDEwLjA0NTYgNi43NzE3OCAxMC4wMjQ3IDYuNzY0ODQgMTAuMDAwOEw0LjIzNDg0IDAuOTk5MTU3QzQuMjI3OSAwLjk3NTM2IDQuMjEzNDIgMC45NTQ0NTEgNC4xOTM1OSAwLjkzOTU3M0M0LjE3Mzc1IDAuOTI0Njk2IDQuMTQ5NjMgMC45MTY2NTYgNC4xMjQ4NCAwLjkxNjY1NkM0LjEwMDA0IDAuOTE2NjU2IDQuMDc1OTIgMC45MjQ2OTYgNC4wNTYwOSAwLjkzOTU3M0M0LjAzNjI1IDAuOTU0NDUxIDQuMDIxNzggMC45NzUzNiA0LjAxNDg0IDAuOTk5MTU3TDIuOTM3NzUgNC44MzA4MkMyLjg4Mzg4IDUuMDIyOTQgMi43Njg4IDUuMTkyMjMgMi42MDk5OCA1LjMxM0MyLjQ1MTE2IDUuNDMzNzcgMi4yNTcyOCA1LjQ5OTQyIDIuMDU3NzUgNS40OTk5OUgwLjkxNjUwNCIgc3Ryb2tlPSIjQTFBMUFBIiBzdHJva2Utd2lkdGg9IjAuOTE2NjY3IiBzdHJva2UtbGluZWNhcD0icm91bmQiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPC9nPgo8ZGVmcz4KPGNsaXBQYXRoIGlkPSJjbGlwMF85OF84MDEiPgo8cmVjdCB3aWR0aD0iMTEiIGhlaWdodD0iMTEiIGZpbGw9IndoaXRlIi8+CjwvY2xpcFBhdGg+CjwvZGVmcz4KPC9zdmc+Cg=="
                                    alt="token"
                                  >
                                  输入Token
                                </h5>
                                <data>4,281</data>
                              </section>
                              <section>
                                <h5>
                                  <img
                                    src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTEiIGhlaWdodD0iMTEiIHZpZXdCb3g9IjAgMCAxMSAxMSIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPGcgY2xpcC1wYXRoPSJ1cmwoI2NsaXAwXzk4XzgwMSkiPgo8cGF0aCBkPSJNMTAuMDgzMiA1LjQ5OTk5SDguOTQ2NUM4Ljc0NjE5IDUuNDk5NTYgOC41NTEyNiA1LjU2NDc1IDguMzkxNTIgNS42ODU2QzguMjMxNzcgNS44MDY0NCA4LjExNiA1Ljk3NjI5IDguMDYxOTIgNi4xNjkxNkw2Ljk4NDg0IDEwLjAwMDhDNi45Nzc5IDEwLjAyNDcgNi45NjM0MiAxMC4wNDU2IDYuOTQzNTkgMTAuMDYwNEM2LjkyMzc1IDEwLjA3NTMgNi44OTk2MyAxMC4wODMzIDYuODc0ODQgMTAuMDgzM0M2Ljg1MDA0IDEwLjA4MzMgNi44MjU5MiAxMC4wNzUzIDYuODA2MDkgMTAuMDYwNEM2Ljc4NjI1IDEwLjA0NTYgNi43NzE3OCAxMC4wMjQ3IDYuNzY0ODQgMTAuMDAwOEw0LjIzNDg0IDAuOTk5MTU3QzQuMjI3OSAwLjk3NTM2IDQuMjEzNDIgMC45NTQ0NTEgNC4xOTM1OSAwLjkzOTU3M0M0LjE3Mzc1IDAuOTI0Njk2IDQuMTQ5NjMgMC45MTY2NTYgNC4xMjQ4NCAwLjkxNjY1NkM0LjEwMDA0IDAuOTE2NjU2IDQuMDc1OTIgMC45MjQ2OTYgNC4wNTYwOSAwLjkzOTU3M0M0LjAzNjI1IDAuOTU0NDUxIDQuMDIxNzggMC45NzUzNiA0LjAxNDg0IDAuOTk5MTU3TDIuOTM3NzUgNC44MzA4MkMyLjg4Mzg4IDUuMDIyOTQgMi43Njg4IDUuMTkyMjMgMi42MDk5OCA1LjMxM0MyLjQ1MTE2IDUuNDMzNzcgMi4yNTcyOCA1LjQ5OTQyIDIuMDU3NzUgNS40OTk5OUgwLjkxNjUwNCIgc3Ryb2tlPSIjQTFBMUFBIiBzdHJva2Utd2lkdGg9IjAuOTE2NjY3IiBzdHJva2UtbGluZWNhcD0icm91bmQiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPC9nPgo8ZGVmcz4KPGNsaXBQYXRoIGlkPSJjbGlwMF85OF84MDEiPgo8cmVjdCB3aWR0aD0iMTEiIGhlaWdodD0iMTEiIGZpbGw9IndoaXRlIi8+CjwvY2xpcFBhdGg+CjwvZGVmcz4KPC9zdmc+Cg=="
                                    alt="token"
                                  >输出Token
                                </h5>
                                <data>2,104</data>
                              </section>
                              <section>
                                <h5>
                                  <img
                                    src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTEiIGhlaWdodD0iMTEiIHZpZXdCb3g9IjAgMCAxMSAxMSIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTUuNDk5ODQgMy42NjY2OFYxLjgzMzM0SDMuNjY2NSIgc3Ryb2tlPSIjQTFBMUFBIiBzdHJva2Utd2lkdGg9IjAuOTE2NjY3IiBzdHJva2UtbGluZWNhcD0icm91bmQiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPHBhdGggZD0iTTguMjUwMTYgMy42NjY2NkgyLjc1MDE2QzIuMjQzOTEgMy42NjY2NiAxLjgzMzUgNC4wNzcwNyAxLjgzMzUgNC41ODMzMlY4LjI0OTk5QzEuODMzNSA4Ljc1NjI1IDIuMjQzOTEgOS4xNjY2NiAyLjc1MDE2IDkuMTY2NjZIOC4yNTAxNkM4Ljc1NjQyIDkuMTY2NjYgOS4xNjY4MyA4Ljc1NjI1IDkuMTY2ODMgOC4yNDk5OVY0LjU4MzMyQzkuMTY2ODMgNC4wNzcwNyA4Ljc1NjQyIDMuNjY2NjYgOC4yNTAxNiAzLjY2NjY2WiIgc3Ryb2tlPSIjQTFBMUFBIiBzdHJva2Utd2lkdGg9IjAuOTE2NjY3IiBzdHJva2UtbGluZWNhcD0icm91bmQiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPHBhdGggZD0iTTAuOTE2NTA0IDYuNDE2NjhIMS44MzMxN005LjE2NjUgNi40MTY2OEgxMC4wODMyTTYuODc0ODQgNS45NTgzNFY2Ljg3NTAxTTQuMTI0ODQgNS45NTgzNFY2Ljg3NTAxIiBzdHJva2U9IiNBMUExQUEiIHN0cm9rZS13aWR0aD0iMC45MTY2NjciIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIvPgo8L3N2Zz4K"
                                    alt="token"
                                  >
                                  总成本
                                </h5>
                                <data>$12.00</data>
                              </section>
                            </CardContent>
                          </Card>
                        </section>
                        <Separator class="" />
                        <section class="flex flex-col items-start *:p-0! ">
                          <Button
                            variant="link"
                            class="text-muted-foreground text-xs cursor-pointer"
                          >
                            <img
                              src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTQiIGhlaWdodD0iMTQiIHZpZXdCb3g9IjAgMCAxNCAxNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTEuNzUgMTEuMDgzM0MyLjU0ODEgMTAuNjIyNiAzLjQ1MzQzIDEwLjM4IDQuMzc1IDEwLjM4QzUuMjk2NTcgMTAuMzggNi4yMDE5IDEwLjYyMjYgNyAxMS4wODMzQzcuNzk4MSAxMC42MjI2IDguNzAzNDMgMTAuMzggOS42MjUgMTAuMzhDMTAuNTQ2NiAxMC4zOCAxMS40NTE5IDEwLjYyMjYgMTIuMjUgMTEuMDgzMyIgc3Ryb2tlPSIjNzM3MzczIiBzdHJva2Utd2lkdGg9IjEuMTY2NjciIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCIvPgo8cGF0aCBkPSJNMS43NSAzLjVDMi41NDgxIDMuMDM5MjEgMy40NTM0MyAyLjc5NjYzIDQuMzc1IDIuNzk2NjNDNS4yOTY1NyAyLjc5NjYzIDYuMjAxOSAzLjAzOTIxIDcgMy41QzcuNzk4MSAzLjAzOTIxIDguNzAzNDMgMi43OTY2MyA5LjYyNSAyLjc5NjYzQzEwLjU0NjYgMi43OTY2MyAxMS40NTE5IDMuMDM5MjEgMTIuMjUgMy41IiBzdHJva2U9IiM3MzczNzMiIHN0cm9rZS13aWR0aD0iMS4xNjY2NyIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0xLjc1IDMuNVYxMS4wODMzIiBzdHJva2U9IiM3MzczNzMiIHN0cm9rZS13aWR0aD0iMS4xNjY2NyIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik03IDMuNVYxMS4wODMzIiBzdHJva2U9IiM3MzczNzMiIHN0cm9rZS13aWR0aD0iMS4xNjY2NyIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0xMi4yNSAzLjVWMTEuMDgzMyIgc3Ryb2tlPSIjNzM3MzczIiBzdHJva2Utd2lkdGg9IjEuMTY2NjciIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCIvPgo8L3N2Zz4K"
                              alt="help"
                            >                          
                            帮助文档
                          </Button>
                          <Button
                            variant="link"
                            class="text-muted-foreground text-xs cursor-pointer text-destructive"
                          >
                            <img
                              src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTQiIGhlaWdodD0iMTQiIHZpZXdCb3g9IjAgMCAxNCAxNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPG1hc2sgaWQ9InBhdGgtMS1pbnNpZGUtMV85OF85MDQiIGZpbGw9IndoaXRlIj4KPHBhdGggZD0iTTkuOTE2NSAzLjQ5OTk5SDEyLjgzMzJWNC42NjY2NkgxMS42NjY1VjEyLjI1QzExLjY2NjUgMTIuNTcyMiAxMS40MDUzIDEyLjgzMzMgMTEuMDgzMiAxMi44MzMzSDIuOTE2NUMyLjU5NDM0IDEyLjgzMzMgMi4zMzMxNyAxMi41NzIyIDIuMzMzMTcgMTIuMjVWNC42NjY2NkgxLjE2NjVWMy40OTk5OUg0LjA4MzE3VjEuNzQ5OTlDNC4wODMxNyAxLjQyNzgzIDQuMzQ0MzQgMS4xNjY2NiA0LjY2NjUgMS4xNjY2Nkg5LjMzMzE3QzkuNjU1MzUgMS4xNjY2NiA5LjkxNjUgMS40Mjc4MyA5LjkxNjUgMS43NDk5OVYzLjQ5OTk5Wk0xMC40OTk4IDQuNjY2NjZIMy40OTk4NFYxMS42NjY3SDEwLjQ5OThWNC42NjY2NlpNNS4yNDk4NCA2LjQxNjY2SDYuNDE2NVY5LjkxNjY2SDUuMjQ5ODRWNi40MTY2NlpNNy41ODMxNyA2LjQxNjY2SDguNzQ5ODRWOS45MTY2Nkg3LjU4MzE3VjYuNDE2NjZaTTUuMjQ5ODQgMi4zMzMzMlYzLjQ5OTk5SDguNzQ5ODRWMi4zMzMzMkg1LjI0OTg0WiIvPgo8L21hc2s+CjxwYXRoIGQ9Ik05LjkxNjUgMy40OTk5OUg4Ljc0OTg0VjQuNjY2NjZIOS45MTY1VjMuNDk5OTlaTTEyLjgzMzIgMy40OTk5OUgxMy45OTk4VjIuMzMzMzJIMTIuODMzMlYzLjQ5OTk5Wk0xMi44MzMyIDQuNjY2NjZWNS44MzMzMkgxMy45OTk4VjQuNjY2NjZIMTIuODMzMlpNMTEuNjY2NSA0LjY2NjY2VjMuNDk5OTlIMTAuNDk5OFY0LjY2NjY2SDExLjY2NjVaTTIuMzMzMTcgNC42NjY2NkgzLjQ5OTg0VjMuNDk5OTlIMi4zMzMxN1Y0LjY2NjY2Wk0xLjE2NjUgNC42NjY2NkgtMC4wMDAxNjI4NFY1LjgzMzMySDEuMTY2NVY0LjY2NjY2Wk0xLjE2NjUgMy40OTk5OVYyLjMzMzMySC0wLjAwMDE2Mjg0VjMuNDk5OTlIMS4xNjY1Wk00LjA4MzE3IDMuNDk5OTlWNC42NjY2Nkg1LjI0OTg0VjMuNDk5OTlINC4wODMxN1pNMTAuNDk5OCA0LjY2NjY2SDExLjY2NjVWMy40OTk5OUgxMC40OTk4VjQuNjY2NjZaTTMuNDk5ODQgNC42NjY2NlYzLjQ5OTk5SDIuMzMzMTdWNC42NjY2NkgzLjQ5OTg0Wk0zLjQ5OTg0IDExLjY2NjdIMi4zMzMxN1YxMi44MzMzSDMuNDk5ODRWMTEuNjY2N1pNMTAuNDk5OCAxMS42NjY3VjEyLjgzMzNIMTEuNjY2NVYxMS42NjY3SDEwLjQ5OThaTTUuMjQ5ODQgNi40MTY2NlY1LjI0OTk5SDQuMDgzMTdWNi40MTY2Nkg1LjI0OTg0Wk02LjQxNjUgNi40MTY2Nkg3LjU4MzE3VjUuMjQ5OTlINi40MTY1VjYuNDE2NjZaTTYuNDE2NSA5LjkxNjY2VjExLjA4MzNINy41ODMxN1Y5LjkxNjY2SDYuNDE2NVpNNS4yNDk4NCA5LjkxNjY2SDQuMDgzMTdWMTEuMDgzM0g1LjI0OTg0VjkuOTE2NjZaTTcuNTgzMTcgNi40MTY2NlY1LjI0OTk5SDYuNDE2NVY2LjQxNjY2SDcuNTgzMTdaTTguNzQ5ODQgNi40MTY2Nkg5LjkxNjVWNS4yNDk5OUg4Ljc0OTg0VjYuNDE2NjZaTTguNzQ5ODQgOS45MTY2NlYxMS4wODMzSDkuOTE2NVY5LjkxNjY2SDguNzQ5ODRaTTcuNTgzMTcgOS45MTY2Nkg2LjQxNjVWMTEuMDgzM0g3LjU4MzE3VjkuOTE2NjZaTTUuMjQ5ODQgMi4zMzMzMlYxLjE2NjY2SDQuMDgzMTdWMi4zMzMzMkg1LjI0OTg0Wk01LjI0OTg0IDMuNDk5OTlINC4wODMxN1Y0LjY2NjY2SDUuMjQ5ODRWMy40OTk5OVpNOC43NDk4NCAzLjQ5OTk5VjQuNjY2NjZIOS45MTY1VjMuNDk5OTlIOC43NDk4NFpNOC43NDk4NCAyLjMzMzMySDkuOTE2NVYxLjE2NjY2SDguNzQ5ODRWMi4zMzMzMlpNOS45MTY1IDMuNDk5OTlWNC42NjY2NkgxMi44MzMyVjMuNDk5OTlWMi4zMzMzMkg5LjkxNjVWMy40OTk5OVpNMTIuODMzMiAzLjQ5OTk5SDExLjY2NjVWNC42NjY2NkgxMi44MzMySDEzLjk5OThWMy40OTk5OUgxMi44MzMyWk0xMi44MzMyIDQuNjY2NjZWMy40OTk5OUgxMS42NjY1VjQuNjY2NjZWNS44MzMzMkgxMi44MzMyVjQuNjY2NjZaTTExLjY2NjUgNC42NjY2NkgxMC40OTk4VjEyLjI1SDExLjY2NjVIMTIuODMzMlY0LjY2NjY2SDExLjY2NjVaTTExLjY2NjUgMTIuMjVIMTAuNDk5OEMxMC40OTk4IDExLjkyNzggMTAuNzYxIDExLjY2NjcgMTEuMDgzMiAxMS42NjY3VjEyLjgzMzNWMTRDMTIuMDQ5NyAxNCAxMi44MzMyIDEzLjIxNjUgMTIuODMzMiAxMi4yNUgxMS42NjY1Wk0xMS4wODMyIDEyLjgzMzNWMTEuNjY2N0gyLjkxNjVWMTIuODMzM1YxNEgxMS4wODMyVjEyLjgzMzNaTTIuOTE2NSAxMi44MzMzVjExLjY2NjdDMy4yMzg2NiAxMS42NjY3IDMuNDk5ODQgMTEuOTI3OCAzLjQ5OTg0IDEyLjI1SDIuMzMzMTdIMS4xNjY1QzEuMTY2NSAxMy4yMTY1IDEuOTUwMDIgMTQgMi45MTY1IDE0VjEyLjgzMzNaTTIuMzMzMTcgMTIuMjVIMy40OTk4NFY0LjY2NjY2SDIuMzMzMTdIMS4xNjY1VjEyLjI1SDIuMzMzMTdaTTIuMzMzMTcgNC42NjY2NlYzLjQ5OTk5SDEuMTY2NVY0LjY2NjY2VjUuODMzMzJIMi4zMzMxN1Y0LjY2NjY2Wk0xLjE2NjUgNC42NjY2NkgyLjMzMzE3VjMuNDk5OTlIMS4xNjY1SC0wLjAwMDE2Mjg0VjQuNjY2NjZIMS4xNjY1Wk0xLjE2NjUgMy40OTk5OVY0LjY2NjY2SDQuMDgzMTdWMy40OTk5OVYyLjMzMzMySDEuMTY2NVYzLjQ5OTk5Wk00LjA4MzE3IDMuNDk5OTlINS4yNDk4NFYxLjc0OTk5SDQuMDgzMTdIMi45MTY1VjMuNDk5OTlINC4wODMxN1pNNC4wODMxNyAxLjc0OTk5SDUuMjQ5ODRDNS4yNDk4NCAyLjA3MjE2IDQuOTg4NjcgMi4zMzMzMiA0LjY2NjUgMi4zMzMzMlYxLjE2NjY2Vi0xLjAyNTJlLTA1QzMuNzAwMDEgLTEuMDI1MmUtMDUgMi45MTY1IDAuNzgzNDk0IDIuOTE2NSAxLjc0OTk5SDQuMDgzMTdaTTQuNjY2NSAxLjE2NjY2VjIuMzMzMzJIOS4zMzMxN1YxLjE2NjY2Vi0xLjAyNTJlLTA1SDQuNjY2NVYxLjE2NjY2Wk05LjMzMzE3IDEuMTY2NjZWMi4zMzMzMkM5LjAxMSAyLjMzMzMyIDguNzQ5ODQgMi4wNzIxNCA4Ljc0OTg0IDEuNzQ5OTlIOS45MTY1SDExLjA4MzJDMTEuMDgzMiAwLjc4MzUwOSAxMC4yOTk3IC0xLjAyNTJlLTA1IDkuMzMzMTcgLTEuMDI1MmUtMDVWMS4xNjY2NlpNOS45MTY1IDEuNzQ5OTlIOC43NDk4NFYzLjQ5OTk5SDkuOTE2NUgxMS4wODMyVjEuNzQ5OTlIOS45MTY1Wk0xMC40OTk4IDQuNjY2NjZWMy40OTk5OUgzLjQ5OTg0VjQuNjY2NjZWNS44MzMzMkgxMC40OTk4VjQuNjY2NjZaTTMuNDk5ODQgNC42NjY2NkgyLjMzMzE3VjExLjY2NjdIMy40OTk4NEg0LjY2NjVWNC42NjY2NkgzLjQ5OTg0Wk0zLjQ5OTg0IDExLjY2NjdWMTIuODMzM0gxMC40OTk4VjExLjY2NjdWMTAuNUgzLjQ5OTg0VjExLjY2NjdaTTEwLjQ5OTggMTEuNjY2N0gxMS42NjY1VjQuNjY2NjZIMTAuNDk5OEg5LjMzMzE3VjExLjY2NjdIMTAuNDk5OFpNNS4yNDk4NCA2LjQxNjY2VjcuNTgzMzJINi40MTY1VjYuNDE2NjZWNS4yNDk5OUg1LjI0OTg0VjYuNDE2NjZaTTYuNDE2NSA2LjQxNjY2SDUuMjQ5ODRWOS45MTY2Nkg2LjQxNjVINy41ODMxN1Y2LjQxNjY2SDYuNDE2NVpNNi40MTY1IDkuOTE2NjZWOC43NDk5OUg1LjI0OTg0VjkuOTE2NjZWMTEuMDgzM0g2LjQxNjVWOS45MTY2NlpNNS4yNDk4NCA5LjkxNjY2SDYuNDE2NVY2LjQxNjY2SDUuMjQ5ODRINC4wODMxN1Y5LjkxNjY2SDUuMjQ5ODRaTTcuNTgzMTcgNi40MTY2NlY3LjU4MzMySDguNzQ5ODRWNi40MTY2NlY1LjI0OTk5SDcuNTgzMTdWNi40MTY2NlpNOC43NDk4NCA2LjQxNjY2SDcuNTgzMTdWOS45MTY2Nkg4Ljc0OTg0SDkuOTE2NVY2LjQxNjY2SDguNzQ5ODRaTTguNzQ5ODQgOS45MTY2NlY4Ljc0OTk5SDcuNTgzMTdWOS45MTY2NlYxMS4wODMzSDguNzQ5ODRWOS45MTY2NlpNNy41ODMxNyA5LjkxNjY2SDguNzQ5ODRWNi40MTY2Nkg3LjU4MzE3SDYuNDE2NVY5LjkxNjY2SDcuNTgzMTdaTTUuMjQ5ODQgMi4zMzMzMkg0LjA4MzE3VjMuNDk5OTlINS4yNDk4NEg2LjQxNjVWMi4zMzMzMkg1LjI0OTg0Wk01LjI0OTg0IDMuNDk5OTlWNC42NjY2Nkg4Ljc0OTg0VjMuNDk5OTlWMi4zMzMzMkg1LjI0OTg0VjMuNDk5OTlaTTguNzQ5ODQgMy40OTk5OUg5LjkxNjVWMi4zMzMzMkg4Ljc0OTg0SDcuNTgzMTdWMy40OTk5OUg4Ljc0OTg0Wk04Ljc0OTg0IDIuMzMzMzJWMS4xNjY2Nkg1LjI0OTg0VjIuMzMzMzJWMy40OTk5OUg4Ljc0OTg0VjIuMzMzMzJaIiBmaWxsPSIjRjg3MTcxIiBtYXNrPSJ1cmwoI3BhdGgtMS1pbnNpZGUtMV85OF85MDQpIi8+Cjwvc3ZnPgo="
                              alt="delete"
                            >                           
                            删除 Session
                          </Button>
                        </section>
                      </section>
                    </form>
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </SidebarContent>
          </Sidebar>
        </SidebarInset>
      </SidebarProvider>
    </Teleport>
    <section class="hidden-clip-section" />
  </section>
</template>

<script setup lang="ts">
// import { computed } from 'vue'
// import { useI18n } from 'vue-i18n'

// import { useQuery } from '@pinia/colada'
// import { getBotsQuery } from '@memoh/sdk/colada'
// import type { BotsBot } from '@memoh/sdk'
// import { useChatStore } from '@/store/chat-list'
// import { storeToRefs } from 'pinia'
import {
  SidebarHeader,
  SidebarProvider,
  SidebarContent,
  Sidebar,
  SidebarInset,
  Separator,
  SidebarMenu,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  InputGroup,
  InputGroupAddon,  
  InputGroupInput, 
  Card,
  CardContent,
  Button,
  SidebarGroup,
  SidebarGroupContent
} from '@memohai/ui'
import useControlVisibleStatus from '@/utils/useControlVisibleStatus'
import { ref } from 'vue'

const sidebarOpen=ref(true)

// const chatStore = useChatStore()
// const { currentBotId } = storeToRefs(chatStore)

// const { data: botData } = useQuery(getBotsQuery())
// const bots = computed<BotsBot[]>(() => botData.value?.items ?? [])

// const isActive = (id: string) => computed(() => {
//   return currentBotId.value === id
// })

// function handleSelect(bot: BotsBot) {
//   chatStore.selectBot(bot.id)
// }

const mountNode = useControlVisibleStatus()
</script>
