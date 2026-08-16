import type { ReactNode } from 'react'
import Modal from './Modal'
import { useLanguage } from '../context/LanguageContext'

interface Props {
  open: boolean
  title: string
  message: string
  onConfirm: () => void
  onCancel: () => void
  busy?: boolean
  children?: ReactNode
}

export default function ConfirmDialog({ open, title, message, onConfirm, onCancel, busy, children }: Props) {
  const { t } = useLanguage()

  return (
    <Modal open={open} title={title} onClose={busy ? () => {} : onCancel} size="sm">
      <div className="modal-body">
        <p>{message}</p>
        {children}
      </div>
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onCancel} disabled={busy}>{t('btn_cancel')}</button>
        <button type="button" className="btn btn-danger" onClick={onConfirm} disabled={busy}>
          {busy && <span className="spinner" />}
          {busy ? t('btn_processing') : t('btn_confirm')}
        </button>
      </div>
    </Modal>
  )
}
