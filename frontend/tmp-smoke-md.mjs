import { looksLikeMarkdown, markdownToDelta, matchMarkdownBlockTrigger } from './app/utils/quill-markdown-paste.ts';  
console.log('header', matchMarkdownBlockTrigger('##'));  
console.log('looksLike', looksLikeMarkdown('## Hi'));  
console.log('ops', JSON.stringify(markdownToDelta('## Hi').ops));  
