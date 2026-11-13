// 领域 API 的统一出口（桶文件）：按域拆分后保持既有
// `import { ... } from '@/api/client'`（或相对路径）的引用不变。
// 新代码建议直接从具体域模块导入。
export * from './http';
export * from './users';
export * from './games';
export * from './guides';
export * from './comments';
export * from './community';
export * from './notifications';
export * from './media';
export * from './feed';
export * from './stats';
