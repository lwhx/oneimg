# OneImg API 开发文档（ScriptCat 用户脚本接入版）

> 项目：[`lwhx/oneimg`](https://github.com/lwhx/oneimg)（初春图床）  
> 依据：2026-11-10 仓库 `main` 分支，审查时基线提交 `0f5a2d4ee07ba2615739ffc9974aaccddb44ee72`。文中响应示例为**结构示意**，并非真实服务器实测返回。  
> 目的：为后续开发 **ScriptCat 图片上传脚本**提供接口契约、鉴权方法、错误处理和实现建议。  
> 另有尚未合并的上传限制调整草稿 [PR #21](https://github.com/lwhx/oneimg/pull/21)。**本文件将当前 `main` 与待合并改动分开说明，不默认 PR 已部署。**

## 1. 接口概览

- API 基础地址：`https://img.example.com/api`，将域名替换为你实际部署的 OneImg 域名。
- 推荐 HTTPS。
- JSON 接口使用 `Content-Type: application/json`；上传本地文件使用 `multipart/form-data`。
- 最适合 ScriptCat 的授权方式：`Authorization: Bearer <API_TOKEN>`。
- 该服务不等于 Telegram Bot API。ScriptCat 只访问 OneImg，具体保存到本地、R2、S3 或 TG 由 OneImg 负责。

### 1.1 Token 可调用的接口清单（源码白名单）

| Method | 路径 | scope | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/status` | `image:read` | 检查 Token 有效性及绑定用户 |
| GET | `/api/uploadConfig` | `image:read` | 读取可用存储桶、默认桶、上传限制、已有标签 |
| GET | `/api/buckets/list` | `image:read` | 查看存储桶清单及启停状态（不等于可上传权限） |
| GET | `/api/tags` | `image:read` | 获取已有标签的 `id` 与 `name` |
| GET | `/api/images` | `image:read` | 分页查自己的图片 |
| GET | `/api/images/:id` | `image:read` | 查看自己有权限访问的图片详情/副本状态 |
| POST | `/api/upload` | `upload:write` | 与下一接口相同的上传处理器，仍使用 `images[]` 字段 |
| POST | **`/api/upload/images`** | `upload:write` | **推荐：文件或批量图片上传** |
| POST | **`/api/images/url`** | `upload:write` | **推荐：服务端抓取公开图片 URL** |

**注意：**代码中 Token 的路由 scope 判断还列出了 `POST /api/images/upload`，但实际路由**没有注册**这个路径，不要使用它。

以下操作**不能仅凭 API Token 调用**：`GET/POST /api/folders`、`DELETE /api/images/:id`、`POST /api/tags`、`POST /api/settings/regenerate`、`/api/uploads/direct/*`（R2 直传任务接口）及其他设置/管理接口。它们需要受支持的浏览器登录会话和相应权限，或将来明确改造服务端的 Token scope/路由规则。不要尝试用管理员 Token 绕过该限制。

## 2. API Token 与鉴权

### 2.1 获取 Token 的正确流程

1. 在 OneImg 后台登录有相关权限的账户。
2. 在系统设置中开启 API 功能（配置项 `start_api`）。API 未开启时，已有 Token 也无法通过鉴权。
3. 在系统设置的 API Token 区域点击**生成 Token**，或者在已登录且有 `setting:api` 权限的浏览器会话中调用 `POST /api/settings/regenerate`。
4. **立即保存** `data.api_token`；明文只返回一次，之后不能通过查询设置找回。
5. 将这个 Token 保存在 ScriptCat 的脚本设置中（例如 `GM_setValue`），不要硬编码进公开发布的脚本，不要写入网页 DOM、日志或 URL。

Token 生成接口示例（**必须是具备设置权限的浏览器登录会话**；不能用已有 API Token 调它）：

```http
POST /api/settings/regenerate HTTP/1.1
Cookie: <浏览器实际登录会话 Cookie>
```

响应结构示意：

```json
{
  "code": 200,
  "message": "API 凭据仅显示一次，请妥善保存",
  "data": {
    "api_token": "oi_<仅示例，真实令牌不要分享>",
    "expires_at": "<ISO8601 到期时间>",
    "scopes": "image:read,upload:write"
  }
}
```

**生命周期与危险操作：**当前新签发 Token 的有效期为 **30 天**，过期需重新生成；代码的重新生成操作会撤销数据库中**所有仍处于有效状态的 API 凭据**，而不仅是当前浏览器的一份 Token，因此可能影响其他集成。建议后续脚本提供“检测过期 / 提示重新配置”，不要调用任何设置接口自动轮换。旧版本 Token 可能沿用 `oneimg_token=` 请求头兼容语法，但建议统一采用 Bearer。

### 2.2 所有 Token 请求的请求头

```http
Authorization: Bearer oi_your_token_here
```

服务器同时识别 `Authorization: oneimg_token=<token>`，但 **Bearer 是推荐格式**。无效/过期/未启用 API 返回 401；Token 无接口权限返回 403。即使 Token 持有者在网页中是管理员，**API Token 请求仍按普通用户权限处理**，不能自动获得管理员存储权限或全站图库访问权。

### 2.3 ScriptCat 跨域与 OneImg 的 Origin 限制（重要）

ScriptCat 推荐 `GM_xmlhttpRequest` 或 `GM.xmlHttpRequest`，配合 `@grant` 与 `@connect`。它们可以处理扩展授权的跨域 HTTP 请求；但这**不代表能绕过服务端自己的 Origin 检查**。

当前 OneImg 对所有 `/api/` 下的写操作（包括上传）有额外限制：如果请求携带 `Sec-Fetch-Site: cross-site`，或者携带了**与 OneImg 的 `APP_URL` 不一致的 `Origin`**，会在进入 API Token 校验前返回 403。对于 ScriptCat 的扩展网络请求，需在真实浏览器环境验证这些请求头是否被附带；不能仅凭代码推断一定能跨站上传。

因此：**优先使用扩展提供的 `GM_xmlhttpRequest`，并先做测试上传。** 如果你的 ScriptCat / 浏览器版本产生跨站来源头而被拒绝，正确的后续改法是在 OneImg 服务端为**通过 Token 鉴权的指定上传接口**建立受控放行规则，并做回归测试；不建议为了省事全局关闭同源保护或开放所有站点 CORS。

## 3. 统一返回格式和差异

多数接口成功时：

```json
{ "code": 200, "message": "ok", "data": {} }
```

多数错误响应：

```json
{ "code": 401, "message": "API 凭据无效、已过期或 API 未开启", "data": null }
```

**不要只按 HTTP 状态判断上传是否全部成功。** 文件上传接口可返回 HTTP 200 / `code:200`，但 `data.files` 中仍有个别项目 `success:false`。必须逐张检查。

还要注意：`GET /api/images/:id` 详情接口成功时历史字段是顶层 `msg`，不是 `message`；部分错误返回可能带 `error_code`、`retry_after` 等。客户端读取错误提示时兼容 `message ?? msg`。

## 4. 核心接口详情

### 4.1 获取上传配置（推荐先调用）

```http
GET /api/uploadConfig
Authorization: Bearer <TOKEN>
```

返回数据结构示意：

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "multi_storage_sync": false,
    "max_upload_files": 10,
    "max_file_size": 10485760,
    "allowed_types": ["image/jpeg", "image/png", "image/gif", "image/webp"],
    "tag_max_length": 10,
    "random_image_limit": 20,
    "tags": [{"id": 1, "name": "商品图"}],
    "default_bucket": 1,
    "buckets": [{"id": 1, "name": "本地存储", "type": "default"}],
    "sync_buckets": []
  }
}
```

**字段：**

| 字段 | 用法 |
| --- | --- |
| `max_file_size` | 上传原始文件单张字节限制，前端先校验 |
| `max_upload_files` | 单次 `POST /api/upload/images` 最多文件数 |
| `allowed_types` | 允许的真实图片 MIME；服务器仍会检测实际文件内容 |
| `buckets` | 当前账户可用或同步关联的桶；应结合模式理解 |
| `default_bucket` | 单存储模式默认目标桶 |
| `multi_storage_sync` | `true` 时先落本地、后台异步同步多个存储；不要误认为所有远端都已完成 |
| `local_bucket`、`sync_buckets` | 多存储同步模式下存在，描述本地桶和同步目标 |
| `tags` | 已有标签集合，上传文件时 `tags` 使用**标签名称数组** |

**版本提示：**尚未合并的 [PR #21](https://github.com/lwhx/oneimg/pull/21) 为响应增加了 `max_batch_size`（普通批量请求的文件总字节预算）；脚本应该检测此字段是否存在，而不是要求所有部署都有它。

### 4.2 文件或批量图片上传（最重要）

**接口**：`POST /api/upload/images`（或等价 `POST /api/upload`）  
**鉴权**：`upload:write`  
**类型**：`multipart/form-data`，不要手动写入缺少 boundary 的 `Content-Type`。

| 表单字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `images[]` | File（二进制 multipart part） | 是 | **同名字段重复提交**表示批量文件；单张也使用这个字段名，不能换成 `file` 或 `image` |
| `folder_id` | 十进制非负整数字符串 | 否 | 文件夹 ID；省略等同 `0` 未分类；必须属于当前 Token 所属账户 |
| `bucket_id` | 十进制 ID 字符串 | 否 | **仅单存储模式**用于选择有权限的目标桶；不填由服务器按账户策略选默认桶 |
| `tags` | JSON 字符串 | 否 | 示例：`["商品图","待发布"]`，必须是**已有标签名称**，不是标签 ID；不会自动创建标签 |

命令行示例（请自行替换域名、Token、文件名和桶 ID）：

```bash
curl -X POST 'https://img.example.com/api/upload/images' \
  -H 'Authorization: Bearer oi_your_token_here' \
  -F 'images[]=@./sofa-front.png;type=image/png' \
  -F 'images[]=@./sofa-detail.jpg;type=image/jpeg' \
  -F 'folder_id=0' \
  -F 'bucket_id=1' \
  -F 'tags=["商品图"]'
```

响应结构示意：

```json
{
  "code": 200,
  "message": "上传成功",
  "data": {
    "files": [
      {
        "success": true,
        "id": 123,
        "folder_id": 0,
        "url": "/uploads/2026/11/abc.webp",
        "thumbnail_url": "/uploads/2026/11/thumbnails/abc.webp",
        "storage": "default",
        "filename": "abc.webp",
        "file_size": 186523,
        "thumbnail_size": 12058,
        "mime_type": "image/webp",
        "width": 1200,
        "height": 900,
        "created_at": "2026-11-10 12:00:00"
      },
      {
        "success": false,
        "filename": "sofa-detail.jpg",
        "folder_id": 0,
        "message": "文件上传失败"
      }
    ],
    "count": 1,
    "failed_count": 1,
    "sync_targets": 0
  }
}
```

上述两个文件混合成功的情况下，HTTP 状态**仍是 200**；若全部失败，接口返回 HTTP 400、`code:400`。`id` 可用于查详情；`filename`/`mime_type` 是**处理后的**文件名/格式，未必等于原文件。

**URL 处理规则：**`url` / `thumbnail_url` 可能是 `/uploads/...` 相对路径，也可能是配置了 CDN 或存储直链后的完整 `https://...` URL。ScriptCat 应当用 `new URL(returnedUrl, oneImgBaseUrl).href` 归一化。**不要自己拼接 Telegram 的 file_id 或 R2 对象地址。**

**单存储 vs 多存储：**`multi_storage_sync=false` 时选择 `bucket_id` 可决定目标桶；`true` 时接口先保存到本地，并异步安排副本同步，传入 `bucket_id` 不能覆盖该策略。`sync_targets` 是计划同步目标数量，不等于成功完成数量；需要时通过 `GET /api/images/:id` 查询 `storage_statuses`。

### 4.3 通过 URL 导入图片（上传网页图片时非常方便）

**接口**：`POST /api/images/url`  
**类型**：`application/json`  
**鉴权**：`upload:write`

```json
{
  "url": "https://static.example.org/example.jpg",
  "folder_id": 0,
  "tag_id": "1",
  "bucket_id": "1"
}
```

| 字段 | 说明 |
| --- | --- |
| `url` | 必填；单个公开可直接获取的 HTTP(S) 图片地址；不是 URL 数组 |
| `folder_id` | 可选，默认 0；需属于当前账户 |
| `tag_id` | **已有标签的 ID 字符串**，与文件批量上传中的标签**名称数组**不同；`"0"` 或省略代表不选 |
| `bucket_id` | 单存储模式可选；多存储同步模式不会用它切换目标 |

返回示意（与批量文件上传**不同**）：

```json
{
  "code": 200,
  "message": "URL 图片上传成功",
  "data": {
    "file": {
      "success": true,
      "id": 124,
      "url": "/uploads/2026/11/example.webp",
      "thumbnail_url": "/uploads/2026/11/thumbnails/example.webp",
      "filename": "example.webp",
      "storage": "default",
      "file_size": 73512,
      "folder_id": 0
    }
  }
}
```

**限制：**由 OneImg **服务器**下载来源图片，必须是服务器能直接访问的合法公开 URL；代码对内网 IP 和 metadata 地址作 SSRF 防护；需要网站登录、仅有浏览器 Cookie、`blob:` 或 `data:` 地址的图片不能直接走这个接口。链接若有防盗链、Referer 或鉴权限制，也可能导入失败。请求一次只提交一个 URL；多图使用客户端队列依次请求。

### 4.4 查看可用存储及标签

```http
GET /api/buckets/list
GET /api/tags
Authorization: Bearer <TOKEN>
```

`GET /api/buckets/list` 返回 `data: [{id, name, type, enabled, disabled}, ...]`，包含可能已经禁用的桶，**不要把全部列表当成自己可上传的桶**；选择上传目标应以 `GET /api/uploadConfig` 为准。

`GET /api/tags` 返回：

```json
{ "code": 200, "message": "ok", "data": { "total": 1, "list": [{"id": 1, "name": "商品图"}] } }
```

**建议：**以后 ScriptCat 设置面板把“存储桶”做成动态下拉，使用 `uploadConfig` 的 `buckets`；标签可从 `tags` 读取已有值，不允许用 API Token 自动新增标签。

### 4.5 查询历史图片

```http
GET /api/images?page=1&limit=20&sort_by=created_at&sort_order=desc
Authorization: Bearer <TOKEN>
```

常用查询：

| 参数 | 类型与含义 |
| --- | --- |
| `page` | 页码，从 1 开始，默认 1 |
| `limit` | 每页图片数，1–100，默认 20 |
| `sort_by` | `created_at` / `file_size` / `filename` |
| `sort_order` | `asc` / `desc` |
| `search` | 文件名模糊搜索 |
| `folder_id` | 所属文件夹 ID，`0` 表示未分类；必须有读取权限 |
| `tags` | 标签 ID，用英文逗号分隔，例如 `1,2`；`0` 表示未加标签 |
| `bucket` | 存储桶 ID 筛选 |

成功时 `data` 包含：`images`（数组）、`total`、`page`、`limit`、`total_pages`。列表内的图片字段与上传回执略有不同，例如图片尺寸同样包含 `width` / `height`，但 MIME 字段是 `mimeType`（驼峰命名）；还包含 `tags`、`storage_statuses`。Token 只允许读取其所属账户的图片，不能请求管理员全站图库。

单张详情：

```http
GET /api/images/123
Authorization: Bearer <TOKEN>
```

成功时 `data` 包含图片对象及 `storage_statuses`；`id` 取自上传结果。存储同步的实际状态应通过详情确认。

### 4.6 文件夹管理与删除（API Token 当前不支持）

后台网页登录会话存在 `GET /api/folders`、`POST /api/folders`、`PUT /api/folders/:id` 和 `DELETE /api/images/:id` 等管理接口，但**API Token 白名单中没有这些路由**。如果 ScriptCat 以后需要“输入名称自动创建文件夹”或“一键删图”，需要单独设计受限 Token scope、账户所有权校验和相关后端改造；当前版本不要假定可以直接通过 Token 使用这些接口。

当前阶段最稳妥的方式是在 OneImg 网页后台**先建好文件夹**，把 `folder_id` 配到 ScriptCat 脚本中。

## 5. 图片格式、大小与存储行为

### 5.1 当前 `main` 的限制

| 项目 | 当前代码约束 |
| --- | --- |
| 文件大小默认值 | 10 MiB（实际部署可能不同，优先读取 `/api/uploadConfig`） |
| 普通存储单文件硬上限 | 32 MiB |
| 默认每批上传数 | 10 张 |
| 每批硬上限 | 50 张 |
| 图片单边最大尺寸 | 8192 px |
| 图片总像素上限 | 1600 万像素 |
| 格式 | JPEG、PNG、GIF、WebP；不允许 SVG |
| Telegram 存储驱动 | 每个最终写入的存储对象最大 20 MiB（独立限制） |
| 账户额度 | 默认 10 GiB，可由后台设置或个人额度覆盖 |

上传限制调整草稿 [PR #21](https://github.com/lwhx/oneimg/pull/21) 将**计划**提高为：新部署默认每张 20 MiB / 每批 20 张；普通存储硬上限每张 64 MiB / 每批 100 张；单次 multipart 最多 256 MiB 文件预算；图片长边 16000 px、总像素 2400 万。**在该 PR 合并且重新部署前，不应视为已生效。**原有数据库设置也不会因为升级就强行重置。

### 5.2 图片保真相关注意事项

- 后台默认 `save_webp=true`，原始 JPEG/PNG 可能被转为 WebP；因此 `url`、`filename`、`mime_type` 可能与原图不同。
- 如果重点是上传原始产品图、透明 PNG 或 SKU 文字图，建议在后台检查 `save_webp`、`compress_image`、`watermark_enable`，并在浏览器端避免二次有损压缩。
- 自动缩略图不是原图，不要拿 `thumbnail_url` 当正式商品主图链接。
- TG 单文件 20 MiB 针对**处理及可能加密后的存储对象**，不是保证原图片 20 MiB 就一定能落 TG。
- 上传后生成的 OneImg URL 可通过项目站点或 CDN 访问；若配置了来源白名单、存储加密或防盗链等策略，外链读取可能受到限制。不要把外链视为私密鉴权资源。

## 6. ScriptCat 脚本接入建议

### 6.1 最小元数据声明（按实际站点修改）

```javascript
// ==UserScript==
// @name         OneImg 上传助手（API 接入样例）
// @namespace    https://github.com/lwhx/oneimg
// @version      0.0.1
// @description  OneImg API 接入示例，非完整发布版
// @match        https://www.example.com/*
// @grant        GM_xmlhttpRequest
// @grant        GM_getValue
// @grant        GM_setValue
// @grant        GM_setClipboard
// @connect      img.example.com
// ==/UserScript==
```

注意：`@match` 应限于要使用脚本的网站；`@connect` 为 OneImg **实际域名**。不要随意把公开发行脚本的 `@connect` 开到 `*`。如脚本要下载任意第三方站点的图片再作为 Blob 上传，需要对图片来源域额外申请权限；只调用 OneImg 的服务端 URL 导入接口则不需要向 ScriptCat 授权第三方图源域名。

ScriptCat 官方文档：
- [GM_xmlhttpRequest API](https://docs.scriptcat.org/docs/dev/api)
- [脚本元数据 @grant / @connect](https://docs.scriptcat.org/docs/dev/meta)

### 6.2 通用跨域请求包装器（开发用示例）

```javascript
const ONEIMG_BASE_URL = 'https://img.example.com';

function oneImgRequest(path, { method = 'GET', token, body, json = false } = {}) {
  return new Promise((resolve, reject) => {
    if (!token) return reject(new Error('请先配置 OneImg API Token'));

    const headers = { Authorization: `Bearer ${token}` };
    if (json) headers['Content-Type'] = 'application/json';
    // 发送 FormData 时不要手动设置 Content-Type，需让运行环境带上 boundary。

    GM_xmlhttpRequest({
      method,
      url: new URL(path, ONEIMG_BASE_URL).href,
      headers,
      data: json ? JSON.stringify(body) : body,
      responseType: 'text',
      timeout: 120000,
      onload(resp) {
        let payload;
        try {
          payload = JSON.parse(resp.responseText || '{}');
        } catch {
          return reject(new Error(`服务器返回非 JSON（HTTP ${resp.status}）`));
        }
        if (resp.status < 200 || resp.status >= 300 || payload.code !== 200) {
          return reject(new Error(payload.message || payload.msg || `HTTP ${resp.status}`));
        }
        resolve(payload.data);
      },
      onerror: () => reject(new Error('网络请求失败，请检查 OneImg 域名或 ScriptCat 跨域权限')),
      ontimeout: () => reject(new Error('请求超时；先查询图库，避免重复上传同一图片'))
    });
  });
}
```

这是**调用辅助函数示例**，并不是可直接安装使用的完整脚本。实际使用时，Token 可通过 `GM_getValue('oneimg_token')` 获取，脚本界面保存新 Token 用 `GM_setValue('oneimg_token', value)`。生产版还应支持取消请求、重试策略、进度显示、选中图片和文件名等能力。

### 6.3 网页图片 URL 导入（无需先下载到脚本）

```javascript
async function importImageUrl(imageUrl, token, folderId = 0, tagId = '0', bucketId) {
  const data = await oneImgRequest('/api/images/url', {
    method: 'POST', token, json: true,
    body: {
      url: imageUrl,
      folder_id: folderId,
      tag_id: String(tagId),
      ...(bucketId != null ? { bucket_id: String(bucketId) } : {})
    }
  });
  if (!data?.file?.success) throw new Error('URL 导入未返回成功图片');
  return {
    ...data.file,
    publicUrl: new URL(data.file.url, ONEIMG_BASE_URL).href
  };
}
```

### 6.4 本地 File / Blob 上传

```javascript
async function uploadFilesToOneImg(files, token, { folderId = 0, bucketId, tags = [] } = {}) {
  const form = new FormData();
  for (const file of files) {
    form.append('images[]', file, file.name || 'image.png');
  }
  form.append('folder_id', String(folderId));
  if (bucketId != null) form.append('bucket_id', String(bucketId));
  if (tags.length) form.append('tags', JSON.stringify(tags)); // 已有标签名称列表

  const data = await oneImgRequest('/api/upload/images', {
    method: 'POST', token, body: form
  });
  const filesResult = Array.isArray(data?.files) ? data.files : [];
  const successes = filesResult.filter(x => x.success);
  const failures = filesResult.filter(x => !x.success);
  return {
    successes: successes.map(x => ({ ...x, publicUrl: new URL(x.url, ONEIMG_BASE_URL).href })),
    failures,
    syncTargets: data?.sync_targets ?? 0
  };
}
```

`File` 与 `Blob` 传给 `GM_xmlhttpRequest` 的 `FormData` 行为，需要用实际 ScriptCat 版本测试。完整产品版应在发送前调用 `/api/uploadConfig` 做文件大小、文件类型、批量张数校验。如果已合并 PR #21 且响应含 `max_batch_size`，再执行本批累计字节数校验。

### 6.5 上传成功后可生成的链接格式

假设 `publicUrl` 为 `https://img.example.com/uploads/2026/11/abc.webp`：

```text
图片直链：https://img.example.com/uploads/2026/11/abc.webp
Markdown：![图片](https://img.example.com/uploads/2026/11/abc.webp)
HTML：<img src="https://img.example.com/uploads/2026/11/abc.webp" alt="图片">
```

后续 ScriptCat 可以提供一键复制直链 / Markdown / HTML，支持多张图片链接批量复制；若用户主要在电商后台使用，应优先把**原图 URL**而非缩略图 URL 插入到详情编辑器。

## 7. 错误处理和重试规则

| 现象 | 可能原因 | 建议 |
| --- | --- | --- |
| 401 | Token 错误、失效、超过 30 天、API 关闭 | 提示重新配置，不自动暴露密码 |
| 403 | Token scope 不足、文件夹/桶权限不足，或者请求来源头被拒绝 | 区分 `message`，检查 `Origin` / `Sec-Fetch-Site` 和桶权限 |
| 404 | 路由不存在或图片不存在 | 确认路径；不要使用未注册的 `/api/images/upload` |
| 413 | 请求体过大 | 读取服务器上传配置，减少单文件或分批 |
| 429 | 全局上传槽忙或其他频率限制 | 尊重 `Retry-After`，指数退避，不要并发轰炸 |
| HTTP 200 + 个别 `success:false` | 批量上传部分失败 | 仅对失败项提示重试，成功项不重复提交 |
| HTTP 400 + `data.files` | 本批全部失败 | 展示每个文件的 `message`；不要按“网络失败”无脑重试 |
| HTTP 200 + `sync_targets > 0` | 多存储后台同步尚未完成 | 需要时按 `id` 查询 `storage_statuses` |
| 返回 HTML 而不是 JSON | 地址写错、反向代理拦截、鉴权重定向 | 检查 HTTP 状态和响应头，不把 HTML 当成功 |

上传传输超时可能发生在服务端**已经保存成功但响应没有送达**之后。脚本不应对超时直接无限重试，而应先查询最近上传图片或提示人工核对，降低产生重复文件的风险。

## 8. 后续 ScriptCat 完整产品的建议需求（此处仅设计，不代表已实现）

**优先实现：**

1. 用户配置 OneImg 地址 + API Token，点击“测试连接”调用 `GET /api/user/status` 和 `GET /api/uploadConfig`。
2. 在指定网页的图片元素增加悬浮上传按钮，或通过右键/工具栏选择多张图片；需要注意用户授权和网站的图片访问权限。
3. 默认优先把**公开 HTTP(S) 图片 URL**交给 `/api/images/url`；导入失败时提供明确提示，并在有权限的情况下使用下载 Blob → `/api/upload/images` 的回退路径。
4. 支持拖拽、粘贴、本地文件选择，动态读取并校验最大张数、文件大小及允许格式。
5. 支持从 `uploadConfig` 读取合法存储桶和已有标签；文件夹 ID 初期手动填写，后续如需自动创建文件夹再扩展后端 Token 权限。
6. 展示每张成功/失败结果；支持复制直链、Markdown、HTML，优先使用上传回执中的 `url`。
7. Token 用 ScriptCat 私有脚本存储，不共享到网页上下文；提供手动清除配置和过期提示。
8. 如果未来需要自动调用 R2 直传任务、删除图片、创建文件夹，应先更新 OneImg **Token scope 与路由授权**，不能直接复用当前 Token 调用会话专用接口。

## 9. 源码核对位置

- [路由定义 `backend/routes/routes.go`](https://github.com/lwhx/oneimg/blob/main/backend/routes/routes.go)
- [Token 鉴权、scope 白名单 `backend/middlewares/AuthMiddleware.go`](https://github.com/lwhx/oneimg/blob/main/backend/middlewares/AuthMiddleware.go)
- [API Token 生成 `backend/controllers/auth/AuthActions.go`](https://github.com/lwhx/oneimg/blob/main/backend/controllers/auth/AuthActions.go)
- [Token 30 天与重新生成行为 `backend/utils/authsecurity/tokens.go`](https://github.com/lwhx/oneimg/blob/main/backend/utils/authsecurity/tokens.go)
- [上传入口与配置 `backend/controllers/images/ImageUpload.go`](https://github.com/lwhx/oneimg/blob/main/backend/controllers/images/ImageUpload.go)
- [上传结果 JSON `backend/controllers/images/UploadResult.go`](https://github.com/lwhx/oneimg/blob/main/backend/controllers/images/UploadResult.go)
- [上传结果字段 `backend/interfaces/image.go`](https://github.com/lwhx/oneimg/blob/main/backend/interfaces/image.go)
- [图片 URL 上传 `backend/controllers/images/ImageURLUpload.go`](https://github.com/lwhx/oneimg/blob/main/backend/controllers/images/ImageURLUpload.go)
- [单存储上传 `backend/controllers/images/LegacyUpload.go`](https://github.com/lwhx/oneimg/blob/main/backend/controllers/images/LegacyUpload.go)
- [图片列表接口 `backend/controllers/images/ImageList.go`](https://github.com/lwhx/oneimg/blob/main/backend/controllers/images/ImageList.go)
- [请求来源限制 `backend/middlewares/SecurityMiddleware.go`](https://github.com/lwhx/oneimg/blob/main/backend/middlewares/SecurityMiddleware.go)
- [ScriptCat 官方 API 文档](https://docs.scriptcat.org/docs/dev/api)
- [ScriptCat 元数据文档](https://docs.scriptcat.org/docs/dev/meta)

---

**文档边界：**以上是静态源码审阅总结；未持有你的部署域名和有效 Token，因此未执行服务器端到端实测。接入脚本前应至少验证一次 `GET /api/user/status`、一次 `POST /api/images/url` 和一次 `POST /api/upload/images`，尤其检查跨站 `Origin` 规则与实际 `multipart FormData` 兼容性。
