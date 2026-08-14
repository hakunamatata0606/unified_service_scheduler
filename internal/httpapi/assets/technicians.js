const filter = document.querySelector('#technician-filter');
const technicianRows = document.querySelector('#technicians');
const technicianStatus = document.querySelector('#technician-status');
const technicianDealershipSelect = document.querySelector('#admin-dealership');

function addCell(row, value) {
  const cell = document.createElement('td');
  cell.textContent = value;
  row.append(cell);
}

filter.addEventListener('submit', async (event) => {
  event.preventDefault();
  const params = new URLSearchParams({
    dealershipId: technicianDealershipSelect.value,
    serviceTypeId: filter.elements.serviceTypeId.value,
    from: new Date(filter.elements.from.value).toISOString(),
    to: new Date(filter.elements.to.value).toISOString(),
  });
  technicianStatus.textContent = 'Loading...';
  try {
    const response = await fetch(`/api/v1/admin/technicians?${params}`);
    const technicians = await response.json();
    if (!response.ok) throw new Error(technicians.error || response.statusText);
    technicianRows.replaceChildren();
    technicians.forEach((technician) => {
      const row = document.createElement('tr');
      addCell(row, technician.name);
      addCell(row, technician.skills.join(', ') || '-');
      addCell(row, technician.available ? 'Available' : 'Busy / not qualified');
      technicianRows.append(row);
    });
    technicianStatus.textContent = `${technicians.filter((item) => item.available).length} available`;
  } catch (error) {
    technicianStatus.textContent = 'Failed to load';
    technicianRows.innerHTML = `<tr><td colspan="3">${error.message}</td></tr>`;
  }
});
