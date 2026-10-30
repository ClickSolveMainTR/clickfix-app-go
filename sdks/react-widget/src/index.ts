'use client';
import { useCallback, useEffect, useState, useSyncExternalStore } from 'react';
import type { components } from './schema.js';
export type { paths, components } from './schema.js';
type S = components['schemas'];
export type Bootstrap = S['WidgetBootstrap'];
export type Visitor = S['WidgetVisitor'];
export type Conversation = S['WidgetConversation'];
export type MessagePage = S['WidgetMessagePage'];
export type SendInput = S['WidgetSendInput'];
export type IdentifyInput = S['WidgetIdentifyInput'];
export type HelpResult = components['schemas']['EnvelopeWidgetHelp']['data'];
export class WidgetError extends Error {
 constructor(public status:number,public code:string){super(`ClickFix widget ${status}: ${code}`);}
}
export interface WidgetOptions {
 /** Same-origin BFF prefix, usually /api. */
 baseURL?:string; publicId:string; fetch?:typeof fetch; token?:string;
 /** Persist a renewed token using your own storage policy. */
 onToken?:(token:string)=>void;
}
export interface Session {bootstrap:Bootstrap|null;visitor:Visitor|null}
/** Create once per mounted widget. No global storage, DOM access or shared user state. */
export function createWidgetClient(options:WidgetOptions){
 const transport=options.fetch??fetch;
 let token=options.token??'';
 let session:Session={bootstrap:null,visitor:null};
 const listeners=new Set<()=>void>();
 let generation=0;
 const save=(visitor:Visitor,bootstrap:Bootstrap|null)=>{
  token=visitor.token;session={visitor,bootstrap};options.onToken?.(token);for(const fn of listeners)fn();
 };
 const part=(s:string)=>encodeURIComponent(s);
 async function request<T>(path:string,method='GET',body?:unknown,signal?:AbortSignal):Promise<T>{
  const headers:Record<string,string>={Accept:'application/json'};
  if(token)headers.Authorization=`Bearer ${token}`;
  const multipart=body instanceof FormData;
  if(body!==undefined&&!multipart)headers['Content-Type']='application/json';
  const res=await transport(`${(options.baseURL??'/api').replace(/\/$/,'')}/v1/widget/${part(options.publicId)}${path}`,{method,headers,body:body===undefined?undefined:multipart?body:JSON.stringify(body),signal,credentials:'omit',redirect:'error'});
  const json=await res.json() as {data:T;error?:{code?:string}};
  if(!res.ok)throw new WidgetError(res.status,json.error?.code??'HTTP_ERROR');
  return json.data;
 }
 const send=(path:string,input:SendInput,file?:File,signal?:AbortSignal)=>{
  const body={...input,client_id:input.client_id??crypto.randomUUID()};
  if(!file)return request<S['WidgetSent']>(path,'POST',body,signal);
  const form=new FormData();form.set('file',file);
  // The existing multipart API accepts these fields; structured events/profile are JSON-only.
  for(const key of ['text','client_id','name','email','page_url'] as const)if(body[key]!==undefined)form.set(key,String(body[key]));
  if(body.form)form.set('form',JSON.stringify(body.form));
  return request<S['WidgetSent']>(path,'POST',form,signal);
 };
 return {
  subscribe:(fn:()=>void)=>{listeners.add(fn);return()=>{listeners.delete(fn);};},
  getSnapshot:()=>session,
  async init(signal?:AbortSignal){const current=++generation;const out=await request<Bootstrap>('/bootstrap','POST',undefined,signal);if(current===generation)save(out.visitor,out);return out;},
  /** user_hash must come from your server's HMAC-SHA256 helper. */
  async identify(input:IdentifyInput,signal?:AbortSignal){const current=++generation;const out=await request<Visitor>('/identify','POST',input,signal);if(current===generation)save(out,null);return out;},
  reset(){generation++;token='';session={bootstrap:null,visitor:null};options.onToken?.('');for(const fn of listeners)fn();},
  conversations:(signal?:AbortSignal)=>request<{items:Conversation[]}>('/conversations','GET',undefined,signal),
  messages:(conversation:string,cursor:{before?:string;after?:string}={},signal?:AbortSignal)=>{
   const q=new URLSearchParams();if(cursor.before)q.set('before',cursor.before);if(cursor.after)q.set('after',cursor.after);
   return request<MessagePage>(`/conversations/${part(conversation)}/messages?${q}`,'GET',undefined,signal);
  },
  createConversation:(input:SendInput,file?:File,signal?:AbortSignal)=>send('/conversations',input,file,signal),
  sendMessage:(conversation:string,input:SendInput,file?:File,signal?:AbortSignal)=>send(`/conversations/${part(conversation)}/messages`,input,file,signal),
  typing:(conversation:string,typing:boolean,signal?:AbortSignal)=>request<{typing:boolean}>(`/conversations/${part(conversation)}/typing`,'POST',{typing},signal),
  markRead:(conversation:string,signal?:AbortSignal)=>request<{updated:number}>(`/conversations/${part(conversation)}/read`,'POST',undefined,signal),
  helpSearch:(q:string,locale:'tr'|'en'='tr',signal?:AbortSignal)=>request<HelpResult>(`/help/search?${new URLSearchParams({q,locale})}`,'GET',undefined,signal),
  realtimeToken:(signal?:AbortSignal)=>request<S['WidgetRealtimeToken']>('/realtime-token','POST',undefined,signal),
  rate:(conversation:string,score:number,comment='',signal?:AbortSignal)=>request<S['WidgetRating']>(`/conversations/${part(conversation)}/rating`,'POST',{score,comment},signal),
  async updateProfile(input:S['WidgetProfileInput'],signal?:AbortSignal){const current=++generation;const out=await request<Visitor>('/profile','PATCH',input,signal);if(current===generation)save(out,null);return out;},
 };
}
export type WidgetClient=ReturnType<typeof createWidgetClient>;
/** Keep client stable (useState/useMemo); call init explicitly after consent. */
export function useWidget(client:WidgetClient){return {...useSyncExternalStore(client.subscribe,client.getSnapshot,client.getSnapshot),client,init:client.init,identify:client.identify,createConversation:client.createConversation,sendMessage:client.sendMessage,typing:client.typing,helpSearch:client.helpSearch};}
function useResource<T>(load:(signal:AbortSignal)=>Promise<T>){
 const [state,setState]=useState<{data:T|null;error:unknown;loading:boolean;source:typeof load|null}>({data:null,error:null,loading:true,source:null});
 const [revision,setRevision]=useState(0);
 useEffect(()=>{
  const abort=new AbortController();setState({data:null,error:null,loading:true,source:load});
  void load(abort.signal).then(data=>{if(!abort.signal.aborted)setState({data,error:null,loading:false,source:load});},error=>{if(!abort.signal.aborted)setState({data:null,error,loading:false,source:load});});
  return()=>abort.abort();
 },[load,revision]);
 const reload=useCallback(()=>setRevision(v=>v+1),[]);
 return state.source===load ? {...state,reload} : {data:null,error:null,loading:true,reload};
}
export function useConversations(client:WidgetClient){const session=useSyncExternalStore(client.subscribe,client.getSnapshot,client.getSnapshot);return useResource(useCallback(signal=>client.conversations(signal),[client,session]));}
export function useMessages(client:WidgetClient,conversation:string){const session=useSyncExternalStore(client.subscribe,client.getSnapshot,client.getSnapshot);return useResource(useCallback(signal=>client.messages(conversation,{},signal),[client,conversation,session]));}
export function useHelpSearch(client:WidgetClient,q:string,locale:'tr'|'en'='tr'){return useResource(useCallback(signal=>client.helpSearch(q,locale,signal),[client,q,locale]));}
export function useTyping(client:WidgetClient,conversation:string){return useCallback((typing:boolean,signal?:AbortSignal)=>client.typing(conversation,typing,signal),[client,conversation]);}
export { loadClickFix, clickFix, useClickFix, useClickFixEvent } from './script.js';
export type { LoadOptions, ClickFixWidgetState } from './script.js';
export type * from './clickfix.js';
