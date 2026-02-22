export const quote = (content: string) => {
  return `\`${content}\``
}

export const block = (content: string, tag: string = '') => {
  return `\`\`\`${tag}\n${content}\n\`\`\``
}