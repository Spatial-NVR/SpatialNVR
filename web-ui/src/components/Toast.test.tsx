import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '../test/utils'
import { render as rtlRender } from '@testing-library/react'
import { ToastProvider, useToast } from './Toast'

// Test component that uses the toast hook
function TestComponent() {
  const { addToast, toasts, removeToast } = useToast()

  return (
    <div>
      <button onClick={() => addToast('success', 'Test message')}>
        Show Success
      </button>
      <button onClick={() => addToast('error', 'Error message')}>
        Show Error
      </button>
      <button onClick={() => addToast('info', 'Info message')}>
        Show Info
      </button>
      <button onClick={() => addToast('warning', 'Warning message')}>
        Show Warning
      </button>
      <div data-testid="toast-count">{toasts.length}</div>
      {toasts.length > 0 && (
        <button onClick={() => removeToast(toasts[0].id)}>Dismiss First</button>
      )}
    </div>
  )
}

describe('Toast', () => {
  describe('ToastProvider', () => {
    it('provides toast context to children', async () => {
      const { user } = render(
        <ToastProvider>
          <TestComponent />
        </ToastProvider>
      )

      const successBtn = screen.getByText('Show Success')
      await user.click(successBtn)

      await waitFor(() => {
        expect(screen.getByText('Test message')).toBeInTheDocument()
      })
    })

    it('can show success toast', async () => {
      const { user } = render(
        <ToastProvider>
          <TestComponent />
        </ToastProvider>
      )

      await user.click(screen.getByText('Show Success'))

      await waitFor(() => {
        expect(screen.getByText('Test message')).toBeInTheDocument()
      })
    })

    it('can show error toast', async () => {
      const { user } = render(
        <ToastProvider>
          <TestComponent />
        </ToastProvider>
      )

      await user.click(screen.getByText('Show Error'))

      await waitFor(() => {
        expect(screen.getByText('Error message')).toBeInTheDocument()
      })
    })

    it('can show info toast', async () => {
      const { user } = render(
        <ToastProvider>
          <TestComponent />
        </ToastProvider>
      )

      await user.click(screen.getByText('Show Info'))

      await waitFor(() => {
        expect(screen.getByText('Info message')).toBeInTheDocument()
      })
    })

    it('can show warning toast', async () => {
      const { user } = render(
        <ToastProvider>
          <TestComponent />
        </ToastProvider>
      )

      await user.click(screen.getByText('Show Warning'))

      await waitFor(() => {
        expect(screen.getByText('Warning message')).toBeInTheDocument()
      })
    })

    it('can show multiple toasts', async () => {
      const { user } = render(
        <ToastProvider>
          <TestComponent />
        </ToastProvider>
      )

      await user.click(screen.getByText('Show Success'))
      await user.click(screen.getByText('Show Error'))

      await waitFor(() => {
        expect(screen.getByText('Test message')).toBeInTheDocument()
        expect(screen.getByText('Error message')).toBeInTheDocument()
        expect(screen.getByTestId('toast-count')).toHaveTextContent('2')
      })
    })

    it('can remove toasts', async () => {
      const { user } = render(
        <ToastProvider>
          <TestComponent />
        </ToastProvider>
      )

      await user.click(screen.getByText('Show Success'))

      await waitFor(() => {
        expect(screen.getByText('Test message')).toBeInTheDocument()
      })

      const dismissBtn = screen.getByText('Dismiss First')
      await user.click(dismissBtn)

      await waitFor(() => {
        expect(screen.queryByText('Test message')).not.toBeInTheDocument()
      })
    })
  })

  describe('useToast hook', () => {
    it('throws error when used outside provider', () => {
      // Create a component that uses useToast outside provider
      function BadComponent() {
        useToast()
        return null
      }

      // Use rtlRender directly without our wrapper (which includes ToastProvider)
      expect(() => {
        rtlRender(<BadComponent />)
      }).toThrow('useToast must be used within a ToastProvider')
    })
  })
})
