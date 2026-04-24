// Thin desktop renderer entry — defers the full bootstrap (Pinia, router,
// i18n, api-client, App.vue) to @memohai/web. When the desktop shell needs
// to diverge, replace this import with an inline copy of web's main.ts and
// customize as needed.
import '@memohai/web/main'
