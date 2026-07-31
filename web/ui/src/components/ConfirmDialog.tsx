import Modal from './Modal'
import { useLanguage } from '../context/LanguageContext'

interface Props {
  open: boolean
  title: string
  message: string
  onConfirm: () => void
  onCancel: () => void
  busy?: boolean
}

export default function ConfirmDialog({ open, title, message, onConfirm, onCancel, busy }: Props) {
  const { t } = useLanguage()

  return (
    <Modal open={open} title={title} onClose={busy ? () => {} : onCancel} size="sm">
      <div className="modal-body">
        <p>{message}</p>
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
