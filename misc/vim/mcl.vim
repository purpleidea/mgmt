" Vim syntax file
" Language: mgmt config language

if exists("b:current_syntax")
	finish
endif

syn case match

syn keyword mclKeywords if else and or not in func class include import as
syn keyword mclTypes bool str int float map struct variant
syn keyword mclBuiltins true false panic

hi def link mclKeywords Keyword
hi def link mclTypes Type
hi def link mclBuiltins Constant

syn keyword mclTodo contained TODO FIXME XXX BUG
syn region mclComment start="#" end="$" contains=mclTodo

hi def link mclTodo Todo
hi def link mclComment Comment

syn keyword mclResources augeas aws:ec2 bmc:power consul:kv cron deploy:tar dhcp:host
syn keyword mclResources dhcp:range dhcp:server docker:container docker:image exec file
syn keyword mclResources firewalld group gzip hetzner:vm hostname http:server:file http:server:flag http:server:proxy
syn keyword mclResources http:server kv mount msg net noop nspawn password pippet pkg print svc
syn keyword mclResources sysctl tar test tftp:file tftp:server timer user value virt virt:builder

hi def link mclResources Constant

syn region mclString start=+"+ skip=+\\"+ end=+"+ contains=mclEscapeChar
syn match mclEscapeChar display contained "\\."

hi def link mclString String
hi def link mclEscapeChar Special

let b:current_syntax = "mcl"
