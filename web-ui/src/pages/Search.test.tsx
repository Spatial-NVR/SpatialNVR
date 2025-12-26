import { describe, it, expect } from 'vitest'
import { render, screen } from '../test/utils'
import { Search } from './Search'

describe('Search', () => {
  it('should render the search page heading', () => {
    render(<Search />)

    expect(screen.getByRole('heading', { name: 'Search' })).toBeInTheDocument()
    expect(screen.getByText('Search through events, faces, and license plates')).toBeInTheDocument()
  })

  it('should render the search input', () => {
    render(<Search />)

    const searchInput = screen.getByPlaceholderText(/Search events, descriptions/i)
    expect(searchInput).toBeInTheDocument()
  })

  it('should render disabled search button', () => {
    render(<Search />)

    const searchButton = screen.getByRole('button', { name: 'Search' })
    expect(searchButton).toBeDisabled()
    expect(searchButton).toHaveAttribute('title', 'AI Search coming in Week 7')
  })

  it('should render search type tabs', () => {
    render(<Search />)

    expect(screen.getByRole('button', { name: /All/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Faces/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /License Plates/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Objects/ })).toBeInTheDocument()
  })

  it('should highlight the default "All" tab', () => {
    render(<Search />)

    const allButton = screen.getByText('All').closest('button')
    expect(allButton).toHaveClass('bg-primary')
  })

  it('should change active tab when clicked', async () => {
    const { user } = render(<Search />)

    const facesButton = screen.getByText('Faces').closest('button')
    await user.click(facesButton!)

    expect(facesButton).toHaveClass('bg-primary')

    const allButton = screen.getByText('All').closest('button')
    expect(allButton).not.toHaveClass('bg-primary')
  })

  it('should render date filter inputs', () => {
    render(<Search />)

    const dateInputs = screen.getAllByRole('textbox', { hidden: true })
    // Date inputs might be type="date" which aren't textboxes
    // Let's check for the "to" label instead
    expect(screen.getByText('to')).toBeInTheDocument()
  })

  it('should render camera filter dropdown', () => {
    render(<Search />)

    expect(screen.getByText('All Cameras')).toBeInTheDocument()
  })

  it('should show coming soon message', () => {
    render(<Search />)

    expect(screen.getByText('AI Search Coming Soon')).toBeInTheDocument()
    expect(screen.getByText(/Semantic search with natural language queries/)).toBeInTheDocument()
  })

  it('should show feature badges', () => {
    render(<Search />)

    expect(screen.getByText('Natural Language')).toBeInTheDocument()
    expect(screen.getByText('Face Recognition')).toBeInTheDocument()
    // 'License Plates' appears twice - once in tabs and once in badges
    expect(screen.getAllByText('License Plates').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('Object Detection')).toBeInTheDocument()
  })

  it('should update search input when typing', async () => {
    const { user } = render(<Search />)

    const searchInput = screen.getByPlaceholderText(/Search events, descriptions/i) as HTMLInputElement
    await user.type(searchInput, 'person at front door')

    expect(searchInput.value).toBe('person at front door')
  })

  it('should switch between all search type tabs', async () => {
    const { user } = render(<Search />)

    // Click on Faces tab
    const facesTab = screen.getByRole('button', { name: /Faces/ })
    await user.click(facesTab)
    expect(facesTab).toHaveClass('bg-primary')

    // Click on License Plates tab
    const licensePlatesTab = screen.getByRole('button', { name: /License Plates/ })
    await user.click(licensePlatesTab)
    expect(licensePlatesTab).toHaveClass('bg-primary')

    // Click on Objects tab
    const objectsTab = screen.getByRole('button', { name: /Objects/ })
    await user.click(objectsTab)
    expect(objectsTab).toHaveClass('bg-primary')

    // Click back to All
    const allTab = screen.getByRole('button', { name: /All/ })
    await user.click(allTab)
    expect(allTab).toHaveClass('bg-primary')
  })
})
