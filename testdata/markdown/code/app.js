// js-search-token
const items=document.querySelectorAll(".card li");for(const item of items){item.addEventListener("click",()=>{item.classList.toggle(`active-${item.dataset.id ?? 0}`)})}if(items.length===0){console.warn("no items")}else{console.log(/li$/i.test(items[0].tagName))}
