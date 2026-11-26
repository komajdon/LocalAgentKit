import { useEffect, useRef, useState } from 'react'
import { StartRecording, StopRecording, WhisperAvailable } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

/** Push-to-talk recording and its transcription. */
export function useSpeech(onText: (text: string) => void) {
  const [whisperReady, setWhisperReady] = useState(false)
  const [recording, setRecording] = useState(false)
  const [transcribing, setTranscribing] = useState(false)
  const [whisperStatus, setWhisperStatus] = useState('') // model download progress
  const [sttError, setSttError] = useState('')
  const recordingRef = useRef(false)

  useEffect(() => {
    WhisperAvailable().then(setWhisperReady).catch(() => {})
    const offs = [
      EventsOn('whisper:downloading', (model: string) => setWhisperStatus(`Downloading whisper ${model} model…`)),
      EventsOn('whisper:ready', () => { setWhisperStatus(''); setWhisperReady(true) }),
      EventsOn('whisper:download_error', (msg: string) => { setWhisperStatus(''); setSttError(`Model download failed: ${msg}`) }),
    ]
    return () => offs.forEach(off => off())
  }, [])

  const startRecording = async () => {
    if (recordingRef.current) return
    recordingRef.current = true
    setSttError('')
    try {
      await StartRecording()
      setRecording(true)
    } catch (err: any) {
      recordingRef.current = false
      setSttError(`Could not start recording: ${err?.message ?? err}`)
    }
  }

  const stopRecording = async () => {
    if (!recordingRef.current) return
    recordingRef.current = false
    setRecording(false)
    setTranscribing(true)
    try {
      const text: string = await StopRecording()
      if (text) onText(text)
    } catch (err: any) {
      setSttError(`${err?.message ?? err}`)
    } finally {
      setTranscribing(false)
    }
  }

  return { whisperReady, recording, transcribing, whisperStatus, sttError, setSttError, startRecording, stopRecording }
}
