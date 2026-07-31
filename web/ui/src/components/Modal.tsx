import { useEffect, useRef, type ReactNode } from 'react'
import { useLanguage } from '../context/LanguageContext'

interface Props {
  open: boolean
  title: string
  onClose: () => void
  children: ReactNode
  size?: 'sm' | 'md'
}

export default function Modal({ open, title, onClose, children, size }: Props) {
  const { t } = useLanguage()
  const dialogRef = useRef<HTMLDialogElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    if (open && !dialog.open) {
      dialog.showModal()
      document.body.classList.add('has-modal')
    } else if (!open && dialog.open) {
      dialog.close()
      document.body.classList.remove('has-modal')
    }
  }, [open])

  useEffect(() => {
    return () => {
      const dialog = dialogRef.current
      if (dialog?.open) dialog.close()
      document.body.classList.remove('has-modal')
    }
  }, [])

  function handleCancel(e: React.SyntheticEvent<HTMLDialogElement>) {
    e.preventDefault()
    onCloseRef.current()
  }

  function handleBackdropClick(e: React.MouseEvent<HTMLDialogElement>) {
    if (e.target === dialogRef.current) onCloseRef.current()
  }

  return (
    <dialog
      ref={dialogRef}
      aria-label={title}
      className={`modal-dialog${size === 'sm' ? ' modal-sm' : ''}`}
      onCancel={handleCancel}
      onClick={handleBackdropClick}
    >
      <div className="modal-header">
        <h3>{title}</h3>
        <button type="button" className="icon-btn" onClick={onClose} aria-label={t('btn_close')}>x</button>
      </div>
      {children}
    </dialog>
  )
}
