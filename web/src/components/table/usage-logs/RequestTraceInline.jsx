/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useState } from 'react';
import { Button, Modal, Spin, Typography } from '@douyinfe/semi-ui';
import { API, copy, formatBytes, showSuccess } from '../../../helpers';

const formatDurationMs = (startedAt, endedAt) => {
  const s = Number(startedAt);
  const e = Number(endedAt);
  if (!Number.isFinite(s) || !Number.isFinite(e) || s <= 0 || e <= 0) return '--';
  const ms = Math.max(0, e - s);
  if (ms < 1000) return `${ms}ms`;
  const sec = ms / 1000;
  if (sec < 60) return `${sec.toFixed(2)}s`;
  return `${(sec / 60).toFixed(2)}m`;
};

const prettyJSONMaybe = (raw) => {
  const text = typeof raw === 'string' ? raw.trim() : '';
  if (!text) return '';
  try {
    const obj = JSON.parse(text);
    return JSON.stringify(obj, null, 2);
  } catch {
    return raw;
  }
};

const RequestTraceInline = ({ requestId, t }) => {
  const rid = useMemo(() => String(requestId || '').trim(), [requestId]);

  const [loading, setLoading] = useState(false);
  const [trace, setTrace] = useState(null);
  const [errMsg, setErrMsg] = useState('');
  const [objectTextByKey, setObjectTextByKey] = useState({});
  const [objectLoadingByKey, setObjectLoadingByKey] = useState({});
  const [objectErrByKey, setObjectErrByKey] = useState({});
  const [objectOpenByKey, setObjectOpenByKey] = useState({});

  const nodes = useMemo(() => {
    const value = trace?.nodes;
    return Array.isArray(value) ? value.filter(Boolean) : [];
  }, [trace]);

  const openRawObjectInNewTab = (e, obj) => {
    if (e?.stopPropagation) e.stopPropagation();
    const url = typeof obj?.download_url === 'string' ? obj.download_url : '';
    if (!url) return;
    window.open(url, '_blank', 'noopener,noreferrer');
  };

  const loadObjectText = async (obj, forceReload = false) => {
    const key = typeof obj?.key === 'string' ? obj.key.trim() : '';
    const url = typeof obj?.download_url === 'string' ? obj.download_url : '';
    if (!key || !url) return;

    if (!forceReload && typeof objectTextByKey[key] === 'string') return;
    if (objectLoadingByKey[key]) return;

    setObjectLoadingByKey((prev) => ({ ...prev, [key]: true }));
    setObjectErrByKey((prev) => ({ ...prev, [key]: '' }));
    try {
      const res = await API.get(url, { skipErrorHandler: true, responseType: 'text' });
      const text = typeof res?.data === 'string' ? res.data : JSON.stringify(res.data);
      setObjectTextByKey((prev) => ({ ...prev, [key]: text }));
    } catch (e) {
      setObjectErrByKey((prev) => ({ ...prev, [key]: String(e?.message || e || '') }));
    } finally {
      setObjectLoadingByKey((prev) => ({ ...prev, [key]: false }));
    }
  };

  useEffect(() => {
    if (!rid) return;
    let disposed = false;
    setLoading(true);
    setErrMsg('');
    setTrace(null);
    setObjectTextByKey({});
    setObjectLoadingByKey({});
    setObjectErrByKey({});
    setObjectOpenByKey({});

    API.get(`/api/request_trace/${encodeURIComponent(rid)}`, {
      skipErrorHandler: true,
    })
      .then((res) => {
        if (disposed) return;
        const { success, message, data } = res.data || {};
        if (!success) {
          setErrMsg(String(message || t('加载失败')));
          return;
        }
        setTrace(data || null);
      })
      .catch((e) => {
        if (disposed) return;
        setErrMsg(String(e?.message || e || t('加载失败')));
      })
      .finally(() => {
        if (disposed) return;
        setLoading(false);
      });

    return () => {
      disposed = true;
    };
  }, [rid]);

  // Auto-load headers (small) for each node.
  useEffect(() => {
    if (!rid || nodes.length === 0) return;
    nodes.forEach((n) => {
      if (n?.request_headers?.key && n?.request_headers?.download_url) {
        loadObjectText(n.request_headers);
      }
      if (n?.response_headers?.key && n?.response_headers?.download_url) {
        loadObjectText(n.response_headers);
      }
    });
  }, [rid, nodes.length]);

  if (!rid) return null;

  return (
    <div className='w-full max-w-full select-text overflow-x-auto rounded border border-semi-color-border bg-semi-color-bg-0 p-2 text-xs'>
      <Spin spinning={loading}>
        <div className='flex flex-col gap-2'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <Typography.Text type='tertiary'>
              {t('请求链路')} ({t('持久化')})
            </Typography.Text>
          </div>

          {errMsg ? <Typography.Text type='danger'>{errMsg}</Typography.Text> : null}

          {!errMsg && nodes.length === 0 ? (
            <Typography.Text type='tertiary'>
              {t('暂无链路数据（可能未开启，或该请求不在采集范围内）')}
            </Typography.Text>
          ) : null}

          {nodes.length > 0 ? (
            <div className='flex flex-col gap-2'>
              {nodes.map((n) => {
                const nodeId = Number(n?.id || 0);
                const service = String(n?.service || '').trim();
                const kind = String(n?.kind || '').trim();
                const seq = Number(n?.seq || 0);
                const method = String(n?.request_method || '').trim();
                const path = String(n?.request_path || '').trim();
                const status = Number(n?.response_status || 0);
                const duration = formatDurationMs(n?.started_at, n?.ended_at);

                const reqHeaders = n?.request_headers;
                const reqBody = n?.request_body;
                const respHeaders = n?.response_headers;
                const respBody = n?.response_body;
                const hasHttpData = reqHeaders?.key || reqBody?.key || respHeaders?.key || respBody?.key;
                const nodeKey = String(nodeId || `${service}:${kind}:${seq}`);

                const renderObjectBlock = (title, obj, opts = {}) => {
                  if (!obj?.key) {
                    return (
                      <div className='flex items-center justify-between gap-2'>
                        <Typography.Text type='tertiary'>{title}</Typography.Text>
                        <Typography.Text type='tertiary'>--</Typography.Text>
                      </div>
                    );
                  }
                  const key = String(obj.key || '').trim();
                  const size = Number(obj.size || 0);
                  const loadingObj = Boolean(objectLoadingByKey[key]);
                  const errObj = String(objectErrByKey[key] || '').trim();
                  const text = objectTextByKey[key];
                  const pretty = opts.pretty ? prettyJSONMaybe(text) : text;
                  const isOpen = Boolean(objectOpenByKey[key]);
                  const hasText = typeof pretty === 'string' && pretty.trim();

                  const toggleOpen = (e) => {
                    if (e?.stopPropagation) e.stopPropagation();
                    const next = !Boolean(objectOpenByKey[key]);
                    setObjectOpenByKey((prev) => ({ ...prev, [key]: next }));
                    if (next && !hasText) {
                      loadObjectText(obj);
                    }
                  };

                  const copyBlock = async (e) => {
                    if (e?.stopPropagation) e.stopPropagation();
                    const textToCopy = typeof pretty === 'string' ? pretty : '';
                    if (!textToCopy.trim()) return;
                    if (await copy(textToCopy)) {
                      showSuccess(t('已复制'));
                    } else {
                      Modal.error({ title: t('无法复制到剪贴板，请手动复制') });
                    }
                  };

                  return (
                    <div className='flex flex-col gap-1 rounded border border-semi-color-border bg-semi-color-bg-1 p-2'>
                      <Typography.Text type='tertiary'>{title}</Typography.Text>
                      <div className='flex flex-wrap items-center gap-2'>
                        <span className='rounded bg-semi-color-bg-2 px-1.5 py-0.5 text-semi-color-text-2'>
                          {formatBytes(size)}
                        </span>
                        {hasText ? (
                          <Button size='small' theme='light' onClick={copyBlock}>
                            {t('复制')}
                          </Button>
                        ) : null}
                        <Button size='small' theme='light' onClick={(e) => openRawObjectInNewTab(e, obj)}>
                          {t('打开')}
                        </Button>
                        <Button size='small' theme='light' loading={loadingObj} onClick={toggleOpen}>
                          {isOpen ? t('收起') : hasText ? t('展开') : t('加载')}
                        </Button>
                      </div>
                      {errObj ? <Typography.Text type='danger'>{errObj}</Typography.Text> : null}
                      {isOpen && hasText ? (
                        <pre className='max-h-[320px] overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] select-text'>
                          {pretty}
                        </pre>
                      ) : null}
                    </div>
                  );
                };

                return (
                  <div key={nodeKey} className='select-text rounded border border-semi-color-border bg-semi-color-bg-1 p-2'>
                    <div className='flex flex-wrap items-center gap-2'>
                      <span className='rounded bg-semi-color-bg-2 px-1.5 py-0.5 text-semi-color-text-2'>
                        {service || '--'}/{kind || '--'}#{seq}
                      </span>
                      {method || path ? (
                        <span className='text-semi-color-text-2' style={{ wordBreak: 'break-all' }}>
                          {method} {path}
                        </span>
                      ) : null}
                      <span className={status >= 400 ? 'text-semi-color-danger' : status ? 'text-semi-color-success' : 'text-semi-color-text-2'}>
                        {status || '--'}
                      </span>
                      <span className='text-semi-color-text-2'>{duration}</span>
                    </div>
                    {(n?.error || n?.request_url || hasHttpData) ? (
                      <div className='mt-2 flex flex-col gap-2'>
                        {n?.error ? (
                          <Typography.Text type='danger'>
                            {t('错误')}: {String(n.error)}
                          </Typography.Text>
                        ) : null}
                        {n?.request_url ? (
                          <Typography.Text type='tertiary' style={{ wordBreak: 'break-all' }}>
                            URL: {String(n.request_url)}
                          </Typography.Text>
                        ) : null}
                        {hasHttpData ? (
                          <div className='grid grid-cols-1 gap-2 lg:grid-cols-2'>
                            <div className='flex flex-col gap-2'>
                              {renderObjectBlock(t('请求Headers'), reqHeaders, { pretty: true })}
                              {renderObjectBlock(t('请求Body'), reqBody, { pretty: false })}
                            </div>
                            <div className='flex flex-col gap-2'>
                              {renderObjectBlock(t('响应Headers'), respHeaders, { pretty: true })}
                              {renderObjectBlock(t('响应Body'), respBody, { pretty: false })}
                            </div>
                          </div>
                        ) : null}
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
          ) : null}
        </div>
      </Spin>
    </div>
  );
};

export default RequestTraceInline;
