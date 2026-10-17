import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import { useAuth } from '../auth/AuthProvider'
import { ApiError, getAttendanceSessions, getAttendanceSections, getStudentAttendanceRoster, getStudentAttendanceMonthly, getOwnStudentAttendanceReport, getGuardianStudentAttendanceReport, recordStudentAttendance, correctStudentAttendance, scanStudentAttendance, getAbsenceEvents, type StudentAttendanceRosterItem, type StudentAttendanceRecord } from '../api/client'

const field = 'mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900'
const card = 'min-w-0 rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:p-6'
const today = new Intl.DateTimeFormat('sv-SE', {timeZone:'Asia/Dhaka',year:'numeric',month:'2-digit',day:'2-digit'}).format(new Date())
type Status = StudentAttendanceRecord['status']
type Draft = { status: Status | ''; note: string }
type Detector = { detect: (source: HTMLVideoElement) => Promise<Array<{rawValue:string}>> }
type DetectorConstructor = new (options: {formats:string[]}) => Detector

export function StudentAttendancePage({permissions}:{permissions:string[]}) {
 const {t,i18n}=useTranslation()
 const {accessToken,renew,expire}=useAuth()
 const cache=useQueryClient()
 const [sessionId,setSessionId]=useState('')
 const [sectionId,setSectionId]=useState('')
 const [date,setDate]=useState(today)
 const [month,setMonth]=useState(today.slice(0,7))
 const [search,setSearch]=useState('')
 const [page,setPage]=useState(1)
 const [reportPage,setReportPage]=useState(1)
 const [alertPage,setAlertPage]=useState(1)
 const [drafts,setDrafts]=useState<Record<string,Draft>>({})
 const [qr,setQr]=useState('')
 const [message,setMessage]=useState('')
 const [cameraError,setCameraError]=useState('')
 const [cameraOn,setCameraOn]=useState(false)
 const video=useRef<HTMLVideoElement>(null)
 const canManage=permissions.includes('attendance:students_manage')
 const canAlerts=permissions.includes('attendance:alerts_read')
 async function authorized<T>(fn:(token:string)=>Promise<T>):Promise<T>{
  if(!accessToken)throw new ApiError(401,'unauthorized')
  try{return await fn(accessToken)}catch(cause){
   if(cause instanceof ApiError&&cause.status===401){try{return await fn(await renew())}catch(retry){if(retry instanceof ApiError&&retry.status===401)expire();throw retry}}
   throw cause
  }
 }
 const sessions=useQuery({queryKey:['attendance','sessions'],queryFn:()=>authorized(token=>getAttendanceSessions(token,new URLSearchParams({page:'1',page_size:'100'}))),retry:false})
 useEffect(()=>{if(!sessionId&&sessions.data?.items.length)setSessionId((sessions.data.items.find(item=>item.is_active)||sessions.data.items.find(item=>item.start_date<=today&&today<=item.end_date)||sessions.data.items[0]).id)},[sessionId,sessions.data])
 useEffect(()=>{setDrafts({});setPage(1)},[sessionId,sectionId,date])
 useEffect(()=>setReportPage(1),[sessionId,sectionId,month,search])
 const sections=useQuery({queryKey:['attendance','sections',sessionId],queryFn:()=>authorized(token=>getAttendanceSections(token,sessionId)),enabled:!!sessionId,retry:false})
 useEffect(()=>{if(sections.data&&!sections.data.some(item=>item.id===sectionId))setSectionId(sections.data[0]?.id||'')},[sections.data,sectionId])
 const rosterParams=new URLSearchParams({session_id:sessionId,section_id:sectionId,date,search,page:String(page),page_size:'10'})
 const roster=useQuery({queryKey:['attendance','student-roster',rosterParams.toString()],queryFn:()=>authorized(token=>getStudentAttendanceRoster(token,rosterParams)),enabled:!!sectionId,retry:false})
 const monthParams=new URLSearchParams({session_id:sessionId,section_id:sectionId,month,search,page:String(reportPage),page_size:'10'})
 const monthly=useQuery({queryKey:['attendance','student-monthly',monthParams.toString()],queryFn:()=>authorized(token=>getStudentAttendanceMonthly(token,monthParams)),enabled:!!sectionId,retry:false})
 const alerts=useQuery({queryKey:['attendance','alerts',alertPage],queryFn:()=>authorized(token=>getAbsenceEvents(token,new URLSearchParams({page:String(alertPage),page_size:'10'}))),enabled:canAlerts,retry:false})
 const label=(status:Status|'')=>t(status?`studentAttendance${status[0].toUpperCase()}${status.slice(1)}`:'attendanceUnmarked')
 async function save(item:StudentAttendanceRosterItem){
  const draft=drafts[item.student_id]||{status:item.status||'',note:item.note||''}
  if(!draft.status)return
  try{
   if(item.record_id)await authorized(token=>correctStudentAttendance(token,item.record_id!,{status:draft.status as Status,note:draft.note}))
   else await authorized(token=>recordStudentAttendance(token,{student_id:item.student_id,attendance_date:date,status:draft.status as Status,note:draft.note}))
   setMessage('studentAttendanceSaved');setDrafts(previous=>{const next={...previous};delete next[item.student_id];return next});await cache.invalidateQueries({queryKey:['attendance']})
  }catch(cause){setMessage(cause instanceof ApiError&&cause.code==='duplicate_attendance'?'studentAttendanceDuplicate':'attendanceSaveError')}
 }
 async function scan(payload:string){
  if(!payload.trim())return
  try{await authorized(token=>scanStudentAttendance(token,{payload:payload.trim(),attendance_date:date}));setQr('');setMessage('studentAttendanceScanned');await cache.invalidateQueries({queryKey:['attendance']})}
  catch(cause){setMessage(cause instanceof ApiError&&cause.code==='duplicate_attendance'?'studentAttendanceDuplicate':'studentAttendanceScanError')}
 }
 useEffect(()=>{
  if(!cameraOn)return
  let cancelled=false,frame=0,stream:MediaStream|undefined
  const ctor=(window as Window & {BarcodeDetector?:DetectorConstructor}).BarcodeDetector
  if(!ctor){setCameraError(t('studentAttendanceCameraUnsupported'));setCameraOn(false);return}
  const detector=new ctor({formats:['qr_code']})
  async function start(){
   try{stream=await navigator.mediaDevices.getUserMedia({video:{facingMode:'environment'}});if(!video.current||cancelled){stream.getTracks().forEach(track=>track.stop());return}video.current.srcObject=stream;await video.current.play()
    async function loop(){if(cancelled||!video.current)return;try{const codes=await detector.detect(video.current);if(codes.length){setCameraOn(false);void scan(codes[0].rawValue);return}}catch{setCameraError(t('studentAttendanceCameraError'))};frame=requestAnimationFrame(()=>void loop())}
    void loop()
   }catch{setCameraError(t('studentAttendanceCameraError'));setCameraOn(false)}
  }
  void start()
  return()=>{cancelled=true;cancelAnimationFrame(frame);stream?.getTracks().forEach(track=>track.stop())}
 },[cameraOn,date,accessToken])
 return <div className="space-y-5">
  <header><h1 className="text-3xl font-bold">{t('studentAttendanceHeading')}</h1><p className="text-slate-600">{t('studentAttendanceDescription')}</p></header>
  {message&&<p role="status" className="rounded-lg bg-teal-50 p-3">{t(message)}</p>}
  <section className={card}><div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
   <label>{t('attendanceSession')}<select className={field} value={sessionId} onChange={event=>{setSessionId(event.target.value);setSectionId('')}}><option value="">—</option>{sessions.data?.items.map(item=><option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
   <label>{t('studentAttendanceSection')}<select className={field} value={sectionId} onChange={event=>{setSectionId(event.target.value);setPage(1)}}><option value="">—</option>{sections.data?.map(item=><option key={item.id} value={item.id}>{item.class_name} / {item.name}</option>)}</select></label>
   <label>{t('attendanceDate')}<input className={field} type="date" value={date} onChange={event=>setDate(event.target.value)} /></label>
   <label>{t('studentAttendanceSearch')}<input className={field} value={search} onChange={event=>{setSearch(event.target.value);setPage(1)}} /></label>
  </div>{(sessions.isError||sections.isError)&&<p role="alert">{t('attendanceLoadError')}</p>}</section>
  {canManage&&<section className={card}><h2 className="text-xl font-bold">{t('studentAttendanceQR')}</h2><p className="text-sm text-slate-600">{t('studentAttendanceQRHelp')}</p><form className="mt-3 flex flex-col gap-2 sm:flex-row" onSubmit={event=>{event.preventDefault();void scan(qr)}}><label className="flex-1">{t('studentAttendanceQRValue')}<input className={field} value={qr} onChange={event=>setQr(event.target.value)} autoComplete="off" /></label><button className="self-end rounded-lg bg-teal-700 px-4 py-2 text-white" type="submit">{t('studentAttendanceScan')}</button></form><button className="mt-3 rounded-lg border px-3 py-2" onClick={()=>setCameraOn(value=>!value)}>{t(cameraOn?'studentAttendanceStopCamera':'studentAttendanceStartCamera')}</button>{cameraError&&<p role="alert">{cameraError}</p>}{cameraOn&&<video ref={video} playsInline muted className="mt-3 max-h-64 w-full rounded-lg bg-black object-contain" />}</section>}
  <section className={card}><h2 className="text-xl font-bold">{t('attendanceDaily')}</h2>{roster.isPending?<p role="status">{t('attendanceLoading')}</p>:roster.isError?<p role="alert">{t('attendanceLoadError')}</p>:!roster.data?.items.length?<p>{t('studentAttendanceEmpty')}</p>:<><div className="mt-3 space-y-2">{roster.data.items.map(item=>{const draft=drafts[item.student_id];return <article key={item.student_id} className="grid gap-2 rounded-lg bg-slate-50 p-3 sm:grid-cols-[2fr_1fr_2fr_auto] sm:items-end"><div><strong>{i18n.language==='bn'&&item.name_bn?item.name_bn:item.name_en}</strong><p className="text-sm text-slate-600">{item.admission_no} · {item.roll_no??'—'}</p></div><label>{t('attendanceStatus')}<select className={field} disabled={!canManage} value={draft?.status??item.status??''} onChange={event=>setDrafts(previous=>({...previous,[item.student_id]:{status:event.target.value as Status,note:previous[item.student_id]?.note??item.note??''}}))}><option value="">{label('')}</option>{(['present','absent','late'] as const).map(status=><option key={status} value={status}>{label(status)}</option>)}</select></label><label>{t('attendanceNote')}<input className={field} maxLength={500} disabled={!canManage} value={draft?.note??item.note??''} onChange={event=>setDrafts(previous=>({...previous,[item.student_id]:{status:previous[item.student_id]?.status??item.status??'',note:event.target.value}}))}/></label>{canManage&&<button className="rounded-lg bg-teal-700 px-3 py-2 text-white" onClick={()=>void save(item)}>{t('attendanceSave')}</button>}</article>})}</div><div className="mt-3 flex gap-3"><button disabled={page<=1} onClick={()=>setPage(page-1)}>{t('attendancePrevious')}</button><span>{page}</span><button disabled={page*10>=(roster.data?.total||0)} onClick={()=>setPage(page+1)}>{t('attendanceNext')}</button></div></>}</section>
  <section className={card}><h2 className="text-xl font-bold">{t('attendanceMonthly')}</h2><label className="mt-2 block max-w-xs">{t('attendanceMonth')}<input className={field} type="month" value={month} onChange={event=>setMonth(event.target.value)} /></label>{monthly.isPending?<p role="status">{t('attendanceLoading')}</p>:monthly.isError?<p role="alert">{t('attendanceLoadError')}</p>:<><ul className="mt-3 space-y-2">{monthly.data?.items.map(item=><li key={item.student_id} className="rounded-lg bg-slate-50 p-3">{i18n.language==='bn'&&item.name_bn?item.name_bn:item.name_en}: {t('attendancePresent')} {item.present} · {t('attendanceAbsent')} {item.absent} · {t('studentAttendanceLate')} {item.late}</li>)}</ul><div className="mt-3 flex gap-3"><button disabled={reportPage<=1} onClick={()=>setReportPage(reportPage-1)}>{t('attendancePrevious')}</button><span>{reportPage}</span><button disabled={reportPage*10>=(monthly.data?.total||0)} onClick={()=>setReportPage(reportPage+1)}>{t('attendanceNext')}</button></div></>}</section>
  {canAlerts&&<section className={card}><h2 className="text-xl font-bold">{t('studentAttendanceAlerts')}</h2>{alerts.isPending?<p role="status">{t('attendanceLoading')}</p>:alerts.isError?<p role="alert">{t('attendanceLoadError')}</p>:<><ul className="mt-3 space-y-2">{alerts.data?.items.map(item=><li key={item.id} className="rounded-lg bg-amber-50 p-3">{item.attendance_date} · {item.name_en} · {t('studentAttendanceLinkedGuardians')}: {item.guardian_count}</li>)}</ul><div className="mt-3 flex gap-3"><button disabled={alertPage<=1} onClick={()=>setAlertPage(alertPage-1)}>{t('attendancePrevious')}</button><span>{alertPage}</span><button disabled={alertPage*10>=(alerts.data?.total||0)} onClick={()=>setAlertPage(alertPage+1)}>{t('attendanceNext')}</button></div></>}</section>}
 </div>
}

export function StudentAttendanceReportPage({guardian}:{guardian:boolean}){
 const {id}=useParams()
 const {t,i18n}=useTranslation()
 const {accessToken,renew,expire}=useAuth()
 const [month,setMonth]=useState(today.slice(0,7))
 async function authorized<T>(fn:(token:string)=>Promise<T>):Promise<T>{if(!accessToken)throw new ApiError(401,'unauthorized');try{return await fn(accessToken)}catch(cause){if(cause instanceof ApiError&&cause.status===401){try{return await fn(await renew())}catch(retry){if(retry instanceof ApiError&&retry.status===401)expire();throw retry}}throw cause}}
 const report=useQuery({queryKey:['attendance','student-report',guardian,id,month],queryFn:()=>authorized(token=>guardian?getGuardianStudentAttendanceReport(token,id!,month):getOwnStudentAttendanceReport(token,month)),enabled:!guardian||!!id,retry:false})
 return <div className="space-y-4"><h1 className="text-3xl font-bold">{t('studentAttendanceReport')}</h1><section className={card}><label>{t('attendanceMonth')}<input className={field} type="month" value={month} onChange={event=>setMonth(event.target.value)} /></label>{report.isPending?<p role="status">{t('attendanceLoading')}</p>:report.isError?<p role="alert">{t('attendanceLoadError')}</p>:<><h2 className="mt-4 text-xl font-semibold">{i18n.language==='bn'&&report.data?.student.name_bn?report.data.student.name_bn:report.data?.student.name_en}</h2><p>{t('attendancePresent')}: {report.data?.student.present} · {t('attendanceAbsent')}: {report.data?.student.absent} · {t('studentAttendanceLate')}: {report.data?.student.late}</p><ul className="mt-4 space-y-2">{report.data?.items.map(item=><li key={item.id} className="flex flex-wrap justify-between gap-2 rounded-lg bg-slate-50 p-3"><span>{item.attendance_date}</span><strong>{t(`studentAttendance${item.status[0].toUpperCase()}${item.status.slice(1)}`)}</strong></li>)}</ul></>}</section></div>
}
