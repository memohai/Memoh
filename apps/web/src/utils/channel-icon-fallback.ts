const explicitChannelFallbacks: Record<string, string> = {
  cli: 'CLI',
  local: 'LC',
  web: 'Web',
}

export function channelIconFallbackText(channel: string): string {
  const normalized = channel.trim().toLowerCase()
  if (!normalized) return ''
  const explicit = explicitChannelFallbacks[normalized]
  
  if (explicit) return explicit.slice(0,1).toUpperCase()
  
 
  const parts = normalized.match(/[a-z0-9]+/gi) ?? []
  
  
  if (parts.length > 1) {
    
    return parts.slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('')
  }
  


  return normalized.replace(/[^a-z0-9]/gi, '').slice(0, 2).toUpperCase()
}
